package locale

import (
	"html/template"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func canonicalLanguage(value string) string {
	tags := i18nBundle.LanguageTags()
	requested, _, _ := language.ParseAcceptLanguage(value)
	if len(requested) == 0 {
		requested = []language.Tag{language.AmericanEnglish}
	}
	_, index, _ := language.NewMatcher(tags).Match(requested...)
	return tags[index].String()
}

type localizedHTML struct {
	base  render.HTMLRender
	mu    sync.Mutex
	cache map[string]*template.Template
}

func LocalizedHTMLRenderer(base render.HTMLRender) render.HTMLRender {
	return &localizedHTML{base: base, cache: make(map[string]*template.Template)}
}

func (r *localizedHTML) Instance(name string, data any) render.Render {
	view, ok := r.base.Instance(name, data).(render.HTML)
	if !ok {
		return r.base.Instance(name, data)
	}
	lang := "en-US"
	if h, ok := data.(gin.H); ok {
		if value, ok := h["dui_locale"].(string); ok && value != "" {
			lang = value
		}
	}
	// Canonicalize before caching: arbitrary Accept-Language headers cannot grow
	// this map beyond the finite set of shipped translations.
	lang = canonicalLanguage(lang)
	r.mu.Lock()
	defer r.mu.Unlock()
	t, cached := r.cache[lang]
	if _, production := r.base.(render.HTMLProduction); !cached || !production {
		var err error
		t, err = view.Template.Clone()
		if err != nil {
			panic(err) // The shared base is never executed; only its clones are rendered.
		}
		localizer := i18n.NewLocalizer(i18nBundle, lang)
		t.Funcs(template.FuncMap{"i18n": func(key string, params ...string) string {
			text, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key, TemplateData: createTemplateData(params)})
			if err != nil {
				return key
			}
			return text
		}})
		if production {
			r.cache[lang] = t
		}
	}
	view.Template = t
	return view
}
