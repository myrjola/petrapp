package i18n

// Language represents a supported language.
type Language string

const (
	// English is the English language.
	English Language = "en"
	// Finnish is the Finnish language.
	Finnish Language = "fi"
	// Swedish is the Swedish language.
	Swedish Language = "sv"
)

// DefaultLanguage is the fallback language.
const DefaultLanguage = English

// getTranslations returns the map of translations.
func getTranslations() map[Language]map[string]string {
	return map[Language]map[string]string{
		English: {
			"home.title":            "Petra",
			"home.tagline":          "Personal trainer in your pocket.",
			"home.signin":           "Sign in",
			"home.register":         "Register",
			"home.footer.privacy":   "Privacy & Security",
			"language.picker.label": "Language",
			"language.name.en":      "English",
			"language.name.fi":      "Suomi",
			"language.name.sv":      "Svenska",
		},
		Finnish: {
			"home.title":            "Petra",
			"home.tagline":          "Henkilökohtainen valmentaja taskussasi.",
			"home.signin":           "Kirjaudu",
			"home.register":         "Rekisteröidy",
			"home.footer.privacy":   "Tietosuoja ja turvallisuus",
			"language.picker.label": "Kieli",
			"language.name.en":      "English",
			"language.name.fi":      "Suomi",
			"language.name.sv":      "Svenska",
		},
		Swedish: {
			"home.title":            "Petra",
			"home.tagline":          "Personlig tränare i fickan.",
			"home.signin":           "Logga in",
			"home.register":         "Registrera",
			"home.footer.privacy":   "Integritet och säkerhet",
			"language.picker.label": "Språk",
			"language.name.en":      "English",
			"language.name.fi":      "Suomi",
			"language.name.sv":      "Svenska",
		},
	}
}

// SupportedLanguages returns a list of all supported languages.
func SupportedLanguages() []Language {
	return []Language{English, Finnish, Swedish}
}

// IsSupported checks if a language is supported.
func IsSupported(lang Language) bool {
	_, ok := getTranslations()[lang]
	return ok
}

// Translate returns the translation for the given key in the specified language.
// If the key is not found, it falls back to the default language.
// If still not found, it returns the key itself.
func Translate(lang Language, key string) string {
	translations := getTranslations()

	// Try the requested language.
	var ok bool
	var langTranslations map[string]string
	var translation string

	if langTranslations, ok = translations[lang]; ok {
		if translation, ok = langTranslations[key]; ok {
			return translation
		}
	}

	// Fallback to default language.
	if lang != DefaultLanguage {
		if langTranslations, ok = translations[DefaultLanguage]; ok {
			if translation, ok = langTranslations[key]; ok {
				return translation
			}
		}
	}

	// Return the key itself if no translation found.
	return key
}
