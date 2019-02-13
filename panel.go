package v2ray_ssrpanel_plugin

import (
	"code.cloudfoundry.org/bytefmt"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/robfig/cron"
	"github.com/shirou/gopsutil/load"
	"google.golang.org/grpc"
	"time"
	"v2ray.com/core/common/protocol"
	"v2ray.com/core/common/serial"
	"v2ray.com/core/proxy/vmess"
)

type Panel struct {
	*Config
	handlerServiceClient *HandlerServiceClient
	statsServiceClient   *StatsServiceClient
	db                   *DB
	userModels           []UserModel
	startAt              time.Time
	node                 *Node
}

func NewPanel(gRPCConn *grpc.ClientConn, db *DB, cfg *Config) (*Panel, error) {
	node, err := db.GetNode(cfg.NodeID)
	if err != nil {
		return nil, err
	}

	newErrorf("node[%d] traffic rate %.2f", node.ID, node.TrafficRate).AtDebug().WriteToLog()

	return &Panel{
		Config:               cfg,
		db:                   db,
		handlerServiceClient: NewHandlerServiceClient(gRPCConn, cfg.UserConfig.InboundTag),
		statsServiceClient:   NewStatsServiceClient(gRPCConn),
		startAt:              time.Now(),
		node:                 node,
	}, nil
}

func (p *Panel) Start() {
	doFunc := func() {
		if err := p.do(); err != nil {
			newError("panel#do").Base(err).AtError().WriteToLog()
		}
	}
	doFunc()

	c := cron.New()
	c.AddFunc(fmt.Sprintf("@every %ds", p.CheckRate), doFunc)
	c.Start()
	c.Run()
}

func (p *Panel) do() error {
	var addedUserCount, deletedUserCount, onlineUsers int
	var uplinkTotal, downlinkTotal uint64
	defer func() {
		newErrorf("+ %d users, - %d users, ↓ %s, ↑ %s, online %d",
			addedUserCount, deletedUserCount, bytefmt.ByteSize(downlinkTotal), bytefmt.ByteSize(uplinkTotal), onlineUsers).AtDebug().WriteToLog()
	}()

	p.db.DB.Create(&NodeInfo{
		NodeID: p.NodeID,
		Uptime: time.Now().Sub(p.startAt) / time.Second,
		Load:   getSystemLoad(),
	})

	userTrafficLogs, err := p.getTraffic()
	if err != nil {
		return err
	}
	onlineUsers = len(userTrafficLogs)

	var uVals, dVals string
	var userIDs []uint

	for _, log := range userTrafficLogs {
		uplink := p.mulTrafficRate(log.Uplink)
		downlink := p.mulTrafficRate(log.Downlink)

		uplinkTotal += log.Uplink
		downlinkTotal += log.Downlink

		log.Traffic = bytefmt.ByteSize(uplink+downlink)
		p.db.DB.Create(&log)

		userIDs = append(userIDs, log.UserID)
		uVals += fmt.Sprintf(" WHEN %d THEN u + %d", log.UserID, uplink)
		dVals += fmt.Sprintf(" WHEN %d THEN d + %d", log.UserID, downlink)
	}

	if onlineUsers > 0 {
		p.db.DB.Create(&NodeOnlineLog{
			NodeID:     p.NodeID,
			OnlineUser: onlineUsers,
		})
	}

	if uVals != "" && dVals != "" {
		p.db.DB.Table("user").
			Where("id in (?)", userIDs).
			Updates(map[string]interface{}{
				"u": gorm.Expr(fmt.Sprintf("CASE id %s END", uVals)),
				"d": gorm.Expr(fmt.Sprintf("CASE id %s END", dVals)),
				"t": time.Now().Unix(),
			})
	}

	addedUserCount, deletedUserCount, err = p.syncUser()
	return nil
}

func (p *Panel) getTraffic() (userTrafficLogs []UserTrafficLog, err error) {
	var downlink, uplink uint64
	for _, user := range p.userModels {
		downlink, err = p.statsServiceClient.getUserDownlink(user.Email)
		if err != nil {
			return
		}

		uplink, err = p.statsServiceClient.getUserUplink(user.Email)
		if err != nil {
			return
		}

		if uplink+downlink > 0 {
			userTrafficLogs = append(userTrafficLogs, UserTrafficLog{
				UserID:   user.ID,
				Uplink:   uplink,
				Downlink: downlink,
				NodeID:   p.NodeID,
				Rate:     p.node.TrafficRate,
			})
		}
	}

	return
}

func (p *Panel) mulTrafficRate(traffic uint64) uint64 {
	return uint64(p.node.TrafficRate * float64(traffic))
}

func (p *Panel) syncUser() (addedUserCount, deletedUserCount int, err error) {
	userModels, err := p.db.GetAllUsers()
	if err != nil {
		return 0, 0, err
	}

	// Calculate addition users
	addUserModels := make([]UserModel, 0)
	for _, userModel := range userModels {
		if inUserModels(&userModel, p.userModels) {
			continue
		}

		addUserModels = append(addUserModels, userModel)
	}

	// Calculate deletion users
	delUserModels := make([]UserModel, 0)
	for _, userModel := range p.userModels {
		if inUserModels(&userModel, userModels) {
			continue
		}

		delUserModels = append(delUserModels, userModel)
	}

	// Delete
	for _, userModel := range delUserModels {
		if i := findUserModelIndex(&userModel, p.userModels); i != -1 {
			p.userModels = append(p.userModels[:i], p.userModels[i+1:]...)
			if err = p.handlerServiceClient.DelUser(userModel.Email); err != nil {
				return
			}
			deletedUserCount++
			newErrorf("Deleted user: id=%d, VmessID=%s, Email=%s", userModel.ID, userModel.VmessID, userModel.Email).AtDebug().WriteToLog()
		}
	}

	// Add
	for _, userModel := range addUserModels {
		if err = p.handlerServiceClient.AddUser(p.convertUser(userModel)); err != nil {
			return
		}
		p.userModels = append(p.userModels, userModel)
		addedUserCount++
		newErrorf("Added user: id=%d, VmessID=%s, Email=%s", userModel.ID, userModel.VmessID, userModel.Email).AtDebug().WriteToLog()
	}

	return
}

func (p *Panel) convertUser(userModel UserModel) *protocol.User {
	userCfg := p.UserConfig
	return &protocol.User{
		Level: userCfg.Level,
		Email: userModel.Email,
		Account: serial.ToTypedMessage(&vmess.Account{
			Id:               userModel.VmessID,
			AlterId:          userCfg.AlterID,
			SecuritySettings: userCfg.securityConfig,
		}),
	}
}

func findUserModelIndex(u *UserModel, userModels []UserModel) int {
	for i, user := range userModels {
		if user == *u {
			return i
		}
	}
	return -1
}

func inUserModels(u *UserModel, userModels []UserModel) bool {
	return findUserModelIndex(u, userModels) != -1
}

func getSystemLoad() string {
	stat, err := load.Avg()
	if err != nil {
		return "0.00 0.00 0.00"
	}

	return fmt.Sprintf("%.2f %.2f %.2f", stat.Load1, stat.Load5, stat.Load15)
}
