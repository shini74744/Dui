package locale

import (
	"embed"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"

	"x-ui/logger"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

var (
	i18nBundle   *i18n.Bundle
	LocalizerWeb *i18n.Localizer
	botLocalizer atomic.Pointer[i18n.Localizer]
	bundleOnce   sync.Once
	bundleErr    error
)

type I18nType string

const (
	Bot I18nType = "bot"
	Web I18nType = "web"
)

type SettingService interface {
	GetTgLang() (string, error)
}

func InitLocalizer(i18nFS embed.FS, settingService SettingService) error {
	bundleOnce.Do(func() {
		bundle := i18n.NewBundle(language.MustParse("en-US"))
		bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
		if bundleErr = parseTranslationFiles(i18nFS, bundle); bundleErr != nil {
			return
		}
		i18nBundle = bundle
		LocalizerWeb = i18n.NewLocalizer(bundle, "en-US")
	})
	if bundleErr != nil {
		return bundleErr
	}

	// setup bot locale
	if err := initTGBotLocalizer(settingService); err != nil {
		return err
	}

	return nil
}

func createTemplateData(params []string, seperator ...string) map[string]any {
	var sep string = "=="
	if len(seperator) > 0 {
		sep = seperator[0]
	}

	templateData := make(map[string]any)
	for _, param := range params {
		parts := strings.SplitN(param, sep, 2)
		templateData[parts[0]] = parts[1]
	}

	return templateData
}

func I18n(i18nType I18nType, key string, params ...string) string {
	var localizer *i18n.Localizer

	switch i18nType {
	case "bot":
		localizer = botLocalizer.Load()
	case "web":
		localizer = LocalizerWeb
	default:
		logger.Errorf("Invalid type for I18n: %s", i18nType)
		return ""
	}

	if localizer == nil {
		return key
	}
	templateData := createTemplateData(params)

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: templateData,
	})
	if err != nil {
		logger.Errorf("Failed to localize message: %v", err)
		return ""
	}

	return msg
}

func initTGBotLocalizer(settingService SettingService) error {
	botLang, err := settingService.GetTgLang()
	if err != nil {
		return err
	}

	botLocalizer.Store(i18n.NewLocalizer(i18nBundle, botLang))
	return nil
}

func LocalizerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var lang string

		if cookie, err := c.Request.Cookie("lang"); err == nil {
			lang = cookie.Value
		} else {
			lang = c.GetHeader("Accept-Language")
		}

		localizer := i18n.NewLocalizer(i18nBundle, lang)
		c.Set("dui_locale", canonicalLanguage(lang))
		c.Set("localizer", localizer)
		c.Set("I18n", func(kind I18nType, key string, params ...string) string {
			if kind != Web {
				return I18n(kind, key, params...)
			}
			msg, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key, TemplateData: createTemplateData(params)})
			if err != nil {
				logger.Errorf("Failed to localize message: %v", err)
				return ""
			}
			return msg
		})
		c.Next()
	}
}

func parseTranslationFiles(i18nFS embed.FS, i18nBundle *i18n.Bundle) error {
	err := fs.WalkDir(i18nFS, "translation",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			data, err := i18nFS.ReadFile(path)
			if err != nil {
				return err
			}

			_, err = i18nBundle.ParseMessageFileBytes(data, path)
			return err
		})
	if err != nil {
		return err
	}

	return nil
}
