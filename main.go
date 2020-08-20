package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/kevinburke/twilio-go"
	"golang.org/x/time/rate"
)

// set up ~/.aws dir, with config and credentials files. basic config looks like:
/*
[default]
output = json
region = us-east-1
*/

func makeCall() error {
	const (
		sid  = "ACad3070cb17d26d01a8fbdadb9cd7a37f"
		to   = "+19176086254"
		from = "+19083889127"
	)
	const tokenfile = "twilio.txt"
	token, err := ioutil.ReadFile(tokenfile)
	if err != nil {
		return fmt.Errorf("needs token file %q: %w", tokenfile, err)
	}
	client := twilio.NewClient(sid, strings.TrimSpace(string(token)), nil)
	callURL, err := url.Parse("https://broyojo.com/twilio")
	if err != nil {
		return err
	}
	call, err := client.Calls.MakeCall(from, to, callURL)
	if err != nil {
		return err
	}
	fmt.Println(call)
	return nil
}

func main() {
	check(makeCall())
	return

	limiter := rate.NewLimiter(rate.Every(time.Second), 1)
	sess, err := session.NewSession()
	check(err)
	svc := sns.New(sess)
	const message = "this is a test of david's project"
	numbers := strings.Split("+19176086254,+19175139575,+19087235723", ",")
	for _, n := range numbers {
		log.Printf("sending to %q:\n", n)
		o, err := svc.Publish(&sns.PublishInput{
			PhoneNumber: aws.String(n),
			Message:     aws.String(message),
			MessageAttributes: map[string]*sns.MessageAttributeValue{
				"AWS.SNS.SMS.SMSType": &sns.MessageAttributeValue{
					DataType:    aws.String("String"),
					StringValue: aws.String("Transactional"),
				},
			},
		})
		check(err)
		log.Printf("response: %v\n", o)
		check(limiter.Wait(context.Background()))
	}
}

// need to request limit increase above $1/month:
// https://aws.amazon.com/premiumsupport/knowledge-center/sns-sms-spending-limit-increase/#:~:text=Amazon%20SNS%20limits%20the%20amount,through%20the%20AWS%20Support%20Center.
// https://console.aws.amazon.com/support/home#/case/create?issueType=service-limit-increase&limitType=service-code-sns-text-messaging

func check(e error) {
	if e != nil {
		panic(e)
	}
}
