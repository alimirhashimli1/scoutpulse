module github.com/scoutpulse/football-svc

go 1.24.0

replace github.com/scoutpulse/libs/auth => ../../libs/auth

require (
	github.com/scoutpulse/libs/auth v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
