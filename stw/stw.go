package stw

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/kevinburke/twilio-go"
)

type Credentials struct {
	SID   string
	Token string
}

func (c Credentials) String() string {
	buf, _ := json.Marshal(c)
	return string(buf)
}

func LoadCredentials(svc *s3.S3) (*Credentials, error) {
	resp, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String("drjin"),
		Key:    aws.String("twilio.json"),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	d := json.NewDecoder(resp.Body)
	var c Credentials
	if err := d.Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c Credentials) NewClient() *twilio.Client {
	return twilio.NewClient(c.SID, strings.TrimSpace(string(c.Token)), nil)
}

func MakeCall(client *twilio.Client, from, to, twimlURL string) (*twilio.Call, error) {
	u, err := url.Parse(twimlURL)
	if err != nil {
		return nil, err
	}
	return client.Calls.MakeCall(from, to, u)
}

func SendSMS(client *twilio.Client, from, to, message string) (*twilio.Message, error) {
	return client.Messages.SendMessage(from, to, message, nil)
}
