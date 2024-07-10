package mobius

import (
	"github.com/go-playground/validator/v10"
	"github.com/jhalter/mobius/hotline"
	"gopkg.in/yaml.v3"
	"log"
	"os"
)

func LoadConfig(path string) (*hotline.Config, error) {
	var config hotline.Config

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	validate := validator.New()
	err = validate.Struct(config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
