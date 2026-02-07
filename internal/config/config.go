package config

// Config 配置文件结构体
type Config struct {
	Version string `yaml:"version"`
	Sqlite  struct {
		Db     string `yaml:"db"`
		Prefix string `yaml:"prefix"`
	} `yaml:"sqlite"`
	Log struct {
		Level  string   `yaml:"level"`
		Writer []string `yaml:"writer"`
	} `yaml:"log"`
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		Version: "1.2.0",
		Sqlite: struct {
			Db     string `yaml:"db"`
			Prefix string `yaml:"prefix"`
		}{
			Db:     "data.db1",
			Prefix: "cdpnetool_",
		},
		Log: struct {
			Level  string   `yaml:"level"`
			Writer []string `yaml:"writer"`
		}{
			Level: "debug",
			// file需要在console之前，因为打包后浏览器控制台日志无法写入会影响文件日志
			Writer: []string{"file", "console"},
		},
	}
}
