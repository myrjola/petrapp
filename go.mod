module github.com/myrjola/petrapp

go 1.26.2

require (
	github.com/PuerkitoBio/goquery v1.12.0
	github.com/alexedwards/scs/sqlite3store v0.0.0-20251002162104-209de6e426de
	github.com/alexedwards/scs/v2 v2.9.0
	github.com/descope/virtualwebauthn v1.0.4
	github.com/go-webauthn/webauthn v0.16.1
	github.com/google/go-cmp v0.7.0
	github.com/mattn/go-sqlite3 v1.14.37
	github.com/openai/openai-go/v3 v3.32.0
	github.com/playwright-community/playwright-go v0.5700.1
	github.com/yuin/goldmark v1.7.16
	golang.org/x/sync v0.20.0
)

// wait for my PR to be merged before removing this directive.
replace github.com/descope/virtualwebauthn v1.0.4 => github.com/myrjola/virtualwebauthn v0.0.0-20260317143742-cbddf7bb22e9

require (
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/telemetry v0.0.0-20260316223853-b6b0c46d1ccd // indirect
	golang.org/x/tools v0.43.0 // indirect
	golang.org/x/tools/go/packages/packagestest v0.1.1-deprecated // indirect
	golang.org/x/vuln v1.1.4 // indirect
)

tool golang.org/x/vuln/cmd/govulncheck
