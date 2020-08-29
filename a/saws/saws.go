package saws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
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

func SendEmail(svc *ses.SES, from, to, subject, body string) (*ses.SendEmailOutput, error) {
	content := func(x string) *ses.Content {
		return &ses.Content{
			Charset: aws.String("UTF-8"),
			Data:    aws.String(x),
		}
	}
	addrs := func(a string) (out []*string) {
		out = append(out, aws.String(a))
		return
	}
	return svc.SendEmail(&ses.SendEmailInput{
		Destination: &ses.Destination{
			ToAddresses:  addrs(to),
			CcAddresses:  []*string{},
			BccAddresses: []*string{},
		},
		Message: &ses.Message{
			Subject: content(subject),
			Body: &ses.Body{
				Text: content(body),
			},
		},
		//ReplyToAddresses: addrs(from),
		Source: aws.String(from),
	})
}
