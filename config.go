package v2ray_ssrpanel_plugin

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"v2ray.com/core/common/errors"
	"v2ray.com/core/common/platform"
	"v2ray.com/core/common/protocol"
	"v2ray.com/core/main/confloader"
	json_reader "v2ray.com/ext/encoding/json"
	"v2ray.com/ext/tools/conf"
)

var (
	commandLine = flag.NewFlagSet(os.Args[0]+"-plugin-ssrpanel", flag.ContinueOnError)

	configFile = commandLine.String("config", "", "Config file for V2Ray.")
	_          = commandLine.Bool("version", false, "Show current version of V2Ray.")
	test       = commandLine.Bool("test", false, "Test config file only, without launching V2Ray server.")
	_          = commandLine.String("format", "json", "Format of input file.")
	_          = commandLine.Bool("plugin", false, "True to load plugins.")
)

type UserConfig struct {
	InboundTag     string `json:"inboundTag"`
	Level          uint32 `json:"level"`
	AlterID        uint32 `json:"alterId"`
	SecurityStr    string `json:"securityConfig"`
	securityConfig *protocol.SecurityConfig
}

func (c *UserConfig) UnmarshalJSON(data []byte) error {
	type config UserConfig
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// set default value
	if cfg.SecurityStr == "" {
		cfg.SecurityStr = "AUTO"
	}

	cfg.securityConfig = &protocol.SecurityConfig{
		Type: protocol.SecurityType(protocol.SecurityType_value[strings.ToUpper(cfg.SecurityStr)]),
	}
	*c = UserConfig(cfg)
	return nil
}

type Config struct {
	NodeID             uint         `json:"nodeId"`
	CheckRate          int          `json:"checkRate"`
	MySQL              *MySQLConfig `json:"mysql"`
	UserConfig         *UserConfig  `json:"user"`
	IgnoreEmptyVmessID bool         `json:"ignoreEmptyVmessID"`
	v2rayConfig        *conf.Config
}

func getConfig() (*Config, error) {
	type config struct {
		*conf.Config
		SSRPanel *Config `json:"ssrpanel"`
	}

	configFile := getConfigFilePath()
	configInput, err := confloader.LoadConfig(configFile)
	if err != nil {
		return nil, errors.New("failed to load config: ", configFile).Base(err)
	}
	defer configInput.Close()

	cfg := &config{}
	if err = decodeCommentJSON(configInput, cfg); err != nil {
		return nil, err
	}
	if cfg.SSRPanel != nil {
		cfg.SSRPanel.v2rayConfig = cfg.Config
		if err = checkCfg(cfg.SSRPanel); err != nil {
			return nil, err
		}
	}

	return cfg.SSRPanel, err
}

func checkCfg(cfg *Config) error {

	if cfg.v2rayConfig.Api == nil {
		return errors.New("Api must be set")
	}

	apiTag := cfg.v2rayConfig.Api.Tag
	if len(apiTag) == 0 {
		return errors.New("Api tag can't be empty")
	}

	services := cfg.v2rayConfig.Api.Services
	if !InStr("HandlerService", services) {
		return errors.New("Api service, HandlerService, must be enabled")
	}
	if !InStr("StatsService", services) {
		return errors.New("Api service, StatsService, must be enabled")
	}

	if cfg.v2rayConfig.Stats == nil {
		return errors.New("Stats must be enabled")
	}

	if apiInbound := getInboundConfigByTag(apiTag, cfg.v2rayConfig.InboundConfigs); apiInbound == nil {
		return errors.New(fmt.Sprintf("Miss an inbound tagged %s", apiTag))
	} else if apiInbound.Protocol != "dokodemo-door" {
		return errors.New(fmt.Sprintf("The protocol of inbound tagged %s must be \"dokodemo-door\"", apiTag))
	} else {
		if apiInbound.ListenOn == nil || apiInbound.PortRange == nil {
			return errors.New(fmt.Sprintf("Fields, \"listen\" and \"port\", of inbound tagged %s must be set", apiTag))
		}
	}

	if inbound := getInboundConfigByTag(cfg.UserConfig.InboundTag, cfg.v2rayConfig.InboundConfigs); inbound == nil {
		return errors.New(fmt.Sprintf("Miss an inbound tagged %s", cfg.UserConfig.InboundTag))
	}

	return nil
}

func getInboundConfigByTag(apiTag string, inbounds []conf.InboundDetourConfig) *conf.InboundDetourConfig {
	for _, inbound := range inbounds {
		if inbound.Tag == apiTag {
			return &inbound
		}
	}
	return nil
}

func getConfigFilePath() string {
	if len(*configFile) > 0 {
		return *configFile
	}

	if workingDir, err := os.Getwd(); err == nil {
		configFile := filepath.Join(workingDir, "config.json")
		if fileExists(configFile) {
			return configFile
		}
	}

	if configFile := platform.GetConfigurationPath(); fileExists(configFile) {
		return configFile
	}

	return ""
}

func decodeCommentJSON(reader io.Reader, i interface{}) error {
	jsonContent := bytes.NewBuffer(make([]byte, 0, 10240))
	jsonReader := io.TeeReader(&json_reader.Reader{
		Reader: reader,
	}, jsonContent)
	decoder := json.NewDecoder(jsonReader)
	return decoder.Decode(i)
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir()
}
