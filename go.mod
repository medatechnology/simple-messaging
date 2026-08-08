module github.com/medatechnology/simple-messaging

go 1.23.2

require github.com/medatechnology/goutil v1.2.2

require (
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lithammer/shortuuid/v4 v4.2.0 // indirect
	golang.org/x/crypto v0.37.0 // indirect
)

replace github.com/medatechnology/goutil => ../goutil
