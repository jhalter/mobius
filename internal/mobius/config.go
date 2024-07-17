package mobius

import (
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/jhalter/mobius/hotline"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

var ConfigSearchOrder = []string{
	"config",
	"/usr/local/var/mobius/config",
	"/opt/homebrew/var/mobius/config",
}

func LoadConfig(path string) (*hotline.Config, error) {
	var config hotline.Config

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %v", err)
	}

	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, fmt.Errorf("unmarshal YAML: %v", err)
	}

	validate := validator.New()
	if err = validate.Struct(config); err != nil {
		return nil, fmt.Errorf("validate config: %v", err)
	}

	// If the FileRoot is an absolute path, use it, otherwise treat as a relative path to the config dir.
	if !filepath.IsAbs(config.FileRoot) {
		config.FileRoot = filepath.Join(path, "../", config.FileRoot)
	}

	return &config, nil
}
