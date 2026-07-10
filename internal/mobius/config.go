package mobius

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/jhalter/mobius/hotline"
	"gopkg.in/yaml.v3"
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
	if err = validate.RegisterValidation("bannerext", func(fl validator.FieldLevel) bool {
		filename := fl.Field().String()
		if filename == "" {
			return true // Allow empty since BannerFile is optional
		}
		ext := strings.ToLower(filepath.Ext(filename))
		return ext == ".jpg" || ext == ".jpeg" || ext == ".gif"
	}); err != nil {
		return nil, fmt.Errorf("register validation: %v", err)
	}
	if err = validate.Struct(config); err != nil {
		// Check if this is a BannerFile validation error and provide a better message
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrs {
				if fieldErr.Field() == "BannerFile" && fieldErr.Tag() == "bannerext" {
					return nil, fmt.Errorf("BannerFile must have a .jpg, .jpeg, or .gif extension (got: %s)", config.BannerFile)
				}
			}
		}
		return nil, fmt.Errorf("validate config: %v", err)
	}

	// FileRoot is returned verbatim: it is a path within the selected file store's namespace, so
	// only the caller knows how to resolve it (e.g. against the config dir for the OS backend).
	return &config, nil
}
