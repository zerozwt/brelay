package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Inbounds []InboundConfig `yaml:"inbounds"`
	LogFile  string          `yaml:"log_file"`
	LoginKey string          `yaml:"login_key"`
}

type InboundConfig struct {
	Name        string   `yaml:"name"`
	Addr        string   `yaml:"addr"`
	Filters     []string `yaml:"filters"`
	TlsKeyFile  string   `yaml:"tls_key"`
	TlsPEMFile  string   `yaml:"tls_pem"`
	TlsInsecure bool     `yaml:"tls_insecure"`
}

var gConf ServerConfig

func initConf() bool {
	conf_file := ""
	flag.StringVar(&conf_file, "conf", "", "config file")
	flag.Parse()

	if len(conf_file) == 0 {
		var err error
		conf_file, err = defaultConfFileName()
		if err != nil {
			logger().Printf("get default config file name failed: %v", err)
			return false
		}
	}

	if err := loadConfig(conf_file); err != nil {
		logger().Printf("load config file %s failed: %v", conf_file, err)
		return false
	}
	return true
}

func loadConfig(conf_file string) error {
	file, err := os.Open(conf_file)
	if err != nil {
		return err
	}
	defer file.Close()
	return yaml.NewDecoder(file).Decode(&gConf)
}

func defaultConfFileName() (string, error) {
	exe_path, err := os.Executable()
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Dir(exe_path))
	if err != nil {
		return "", err
	}
	if len(path) == 0 {
		return path, errors.New("directory of current executable file is empty")
	}

	slash := "/"
	if runtime.GOOS == "windows" {
		slash = "\\"
	}
	if path[len(path)-1] != byte(slash[0]) {
		path += slash
	}

	return path + "config.yaml", nil
}
