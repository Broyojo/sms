package saws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
)

// NewSession creates a new authenticated aws session.
func NewSession() (*session.Session, error) {
	return NewSessionFromProfile("")
}

func NewSessionFromProfile(profile string) (*session.Session, error) {
	return session.NewSessionWithOptions(session.Options{
		Profile:           profile,
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			// DisableRestProtocolURICleaning makes s3 keys more true and robust
			DisableRestProtocolURICleaning: aws.Bool(true),
		},
	})
}
