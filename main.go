package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/kevinburke/twilio-go"
	"github.com/xoba/sms/a/jin"
	"github.com/xoba/sms/a/saws"
	"golang.org/x/time/rate"
)

type Config struct {
	Profile  string
	Quantity int
	Hertz    float64
}

func (c Config) AWSSession() (*session.Session, error) {
	return saws.NewSessionFromProfile(c.Profile)
}

func main() {
	var config Config
	flag.StringVar(&config.Profile, "p", "", "aws iam profile to use, if any")
	flag.IntVar(&config.Quantity, "q", 0, "max quantity of folks to reach out to")
	flag.Float64Var(&config.Hertz, "f", 2, "max frequency of contact, hertz")
	flag.Parse()

	if err := Prod(config); err != nil {
		log.Fatal(err)
	}
}

func Prod(c Config) error {
	if c.Hertz > 2 {
		return fmt.Errorf("too fast!")
	}

	svc, err := c.AWSSession()
	if err != nil {
		return err
	}
	info, err := jin.LoadContacts(s3.New(svc))
	if err != nil {
		return err
	}
	var all []jin.Decision
	actions := make(map[string]int)
	preferred := make(map[string]int)
	var noDecisions int
	for _, i := range info {
		preferred[string(i.Preferred)]++
		fmt.Println(i)
		decisions, err := i.Decisions()
		if err != nil {
			return err
		}
		if len(decisions) == 0 {
			noDecisions++
		}
		all = append(all, decisions...)
		for _, d := range decisions {
			fmt.Printf("  %s\n", d)
			actions[d.Type()]++
		}
	}
	pricing := map[string]float64{
		"email": 0.10 / 1000,
		"phone": 0.0130, // per minute
		"sms":   0.0075,
	}
	fmt.Printf("preferences: %v\n", preferred)
	fmt.Printf("%d no-decision contacts, %d total decisions; %v\n", noDecisions, len(all), actions)
	var total float64
	for k, v := range pricing {
		n := actions[k]
		c := v * float64(n)
		total += c
		fmt.Printf("cost for %d %s: $%.2f\n", n, k, c)
	}
	fmt.Printf("total estimated cost: $%.2f\n", total)

	rand.Shuffle(len(all), func(i, j int) {
		all[i], all[j] = all[j], all[i]
	})

	if len(all) > c.Quantity {
		all = all[:c.Quantity]
	}

	dt := time.Duration(1 / c.Hertz * float64(time.Second))
	limiter := rate.NewLimiter(rate.Every(dt), 1)

	alreadyDone := func(d jin.Decision) (bool, error) {
		resp, err := s3.New(svc).GetObject(&s3.GetObjectInput{
			Bucket: aws.String("drjin"),
			Key:    aws.String(path.Join("receipts", d.Key())),
		})
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				return false, nil
			default:
				return false, err
			}
		}

		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		var r jin.Receipt
		if err := dec.Decode(r); err != nil {
			return false, err
		}
		return r.Successful, nil
	}

	markDone := func(r *jin.Receipt) error {
		buf, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := s3.New(svc).PutObject(&s3.PutObjectInput{
			Bucket: aws.String("drjin"),
			Key:    aws.String(path.Join("receipts", r.Decision.Key())),
			Body:   bytes.NewReader(buf),
		}); err != nil {
			return err
		}
		return nil
	}

	for _, d := range all {
		done, err := alreadyDone(d)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		r, err := d.Contact()
		if err != nil {
			return err
		}
		if err := markDone(r); err != nil {
			return err
		}
		if err := limiter.Wait(context.Background()); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func makeCall(to string) error {
	const (
		sid  = "ACad3070cb17d26d01a8fbdadb9cd7a37f"
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
		return fmt.Errorf("can't make call: %w", err)
	}
	dump(call)
	return nil
}

func dump(i interface{}) {
	buf, _ := json.MarshalIndent(i, "", "  ")
	fmt.Println(string(buf))
}

func Run() error {
	var to string
	flag.StringVar(&to, "to", "+19176086254", "phone number to call")
	flag.Parse()

	return makeCall(to)

	limiter := rate.NewLimiter(rate.Every(time.Second), 1)
	sess, err := session.NewSession()
	if err != nil {
		return fmt.Errorf("can't create session: %w", err)
	}
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
		if err != nil {
			return fmt.Errorf("can't publish: %w", err)
		}
		log.Printf("response: %v\n", o)
		if err := limiter.Wait(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

// need to request limit increase above $1/month:
// https://aws.amazon.com/premiumsupport/knowledge-center/sns-sms-spending-limit-increase/#:~:text=Amazon%20SNS%20limits%20the%20amount,through%20the%20AWS%20Support%20Center.
// https://console.aws.amazon.com/support/home#/case/create?issueType=service-limit-increase&limitType=service-code-sns-text-messaging
