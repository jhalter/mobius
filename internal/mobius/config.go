package mobius

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

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

	if err := normalizeNewsFeedConfig(&config); err != nil {
		return nil, err
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

func normalizeNewsFeedConfig(config *hotline.Config) error {
	seenPaths := make(map[string]struct{}, len(config.NewsFeeds))
	for i := range config.NewsFeeds {
		feed := &config.NewsFeeds[i]
		if len(feed.CategoryPath) == 0 {
			return fmt.Errorf("NewsFeeds[%d].CategoryPath is required", i)
		}
		for j, segment := range feed.CategoryPath {
			if segment == "" || !utf8.ValidString(segment) || strings.ContainsRune(segment, '\x00') || len(segment) > 255 {
				return fmt.Errorf("NewsFeeds[%d].CategoryPath[%d] must be non-empty valid UTF-8 without NUL bytes and at most 255 bytes", i, j)
			}
		}

		pathKey := strings.Join(feed.CategoryPath, "\x00")
		if _, exists := seenPaths[pathKey]; exists {
			return fmt.Errorf("NewsFeeds[%d].CategoryPath duplicates another news feed", i)
		}
		seenPaths[pathKey] = struct{}{}

		parsedURL, err := url.Parse(feed.URL)
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return fmt.Errorf("NewsFeeds[%d].URL must be an absolute HTTP or HTTPS URL", i)
		}
		if parsedURL.User != nil {
			return fmt.Errorf("NewsFeeds[%d].URL must not contain credentials", i)
		}
	}

	return nil
}
