package v2ray_ssrpanel_plugin

import (
	"fmt"
	"google.golang.org/grpc/status"
	"os"
	"time"
	"v2ray.com/core/common/errors"
)

func init() {
	go func() {
		err := run()
		if err != nil {
			fatal(err)
		}
	}()
}

func run() error {
	commandLine.Parse(os.Args[1:])

	cfg, err := getConfig()
	if err != nil || *test || cfg == nil {
		return err
	}

	// wait v2ray
	time.Sleep(time.Second)

	db, err := NewMySQLConn(cfg.MySQL)
	if err != nil {
		return err
	}

	go func() {
		apiInbound := getInboundConfigByTag(cfg.v2rayConfig.Api.Tag, cfg.v2rayConfig.InboundConfigs)
		gRPCAddr := fmt.Sprintf("%s:%d", apiInbound.ListenOn.String(), apiInbound.PortRange.From)
		gRPCConn, err := connectGRPC(gRPCAddr, 10*time.Second)
		if err != nil {
			if s, ok := status.FromError(err); ok {
				err = errors.New(s.Message())
			}
			fatal(fmt.Sprintf("connect to gRPC server \"%s\" err: ", gRPCAddr), err)
		}
		newErrorf("Connected gRPC server \"%s\" ", gRPCAddr).AtWarning().WriteToLog()

		p, err := NewPanel(gRPCConn, db, cfg)
		if err != nil {
			fatal("new panel error", err)
		}

		p.Start()
	}()

	return nil
}

func newErrorf(format string, a ...interface{}) *errors.Error {
	return newError(fmt.Sprintf(format, a...))
}

func newError(values ...interface{}) *errors.Error {
	values = append([]interface{}{"SSRPanelPlugin: "}, values...)
	return errors.New(values...)
}

func fatal(values ...interface{}) {
	newError(values...).AtError().WriteToLog()
	// Wait log
	time.Sleep(1 * time.Second)
	os.Exit(-2)
}
