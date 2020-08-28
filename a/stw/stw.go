package stw

import (
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
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
