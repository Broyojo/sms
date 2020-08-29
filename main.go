package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"path"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/xoba/sms/a/jin"
	"github.com/xoba/sms/a/saws"
	"github.com/xoba/sms/a/stw"
	"golang.org/x/time/rate"
)

type Config struct {
	Profile  string
	Quantity int
	Hertz    float64
	Prod     bool
}

func (c Config) AWSSession() (*session.Session, error) {
	return saws.NewSessionFromProfile(c.Profile)
}

func main() {
	var config Config
	flag.BoolVar(&config.Prod, "prod", false, "production mode or not")
	flag.StringVar(&config.Profile, "p", "", "aws iam profile to use, if any")
	flag.IntVar(&config.Quantity, "q", 0, "max quantity of folks to reach out to")
	flag.Float64Var(&config.Hertz, "f", 2, "max frequency of contact, hertz")
	flag.Parse()

	var f func(Config) error
	if config.Prod {
		f = ProdMode
	} else {
		f = DevMode
	}
	if err := f(config); err != nil {
		log.Fatal(err)
	}
}

func ProdMode(c Config) error {
	if c.Hertz > 2 {
		return fmt.Errorf("too fast!")
	}
	session, err := c.AWSSession()
	if err != nil {
		return err
	}
	creds, err := stw.LoadCredentials(s3.New(session))
	if err != nil {
		return err
	}
	info, err := jin.LoadContacts(s3.New(session))
	if err != nil {
		return err
	}
	dumpSortedMap := func(name string, m map[string]int) {
		var list []string
		for k := range m {
			list = append(list, k)
		}
		sort.Slice(list, func(i, j int) bool {
			return m[list[i]] > m[list[j]]
		})
		fmt.Printf("%s:\n", name)
		for _, k := range list {
			fmt.Printf("  %d %q\n", m[k], k)
		}
	}
	var all []jin.Decision
	hosts := make(map[string]int)
	actions := make(map[string]int)
	preferred := make(map[string]int)
	states := make(map[string]int)
	hasAddress := make(map[bool]int)
	hasEmail := make(map[bool]int)
	var noDecisions int
	for _, i := range info {
		hasAddress[i.HasAddress()]++
		hasEmail[i.Email != ""]++
		states[i.State]++
		preferred[string(i.Preferred)]++
		hosts[i.Host()]++
		fmt.Println(i)
		if i.Email != "" {
			if err := i.ValidateEmail(); err != nil {
				return err
			}
		}
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
	if false {
		/*
			"debvoise.com" not found
			"solusp.com" not found
			"" not found
			"iclowd.com" not found --- icloud.com?
			"neus.sr" not found
			"maine.eudu" not found --- maine.edu?
			"no.com" not found
			"ail.com" not found --- mail.com?
			"dfkdf.com" not found
			"dreierllp.com" not found
			"nyc.rrcom" not found --- nyc.rr.com?
		*/
		for h := range hosts {
			_, err := net.LookupMX(h)
			if err != nil {
				if nerr, ok := err.(*net.DNSError); ok {
					if nerr.IsNotFound {
						fmt.Printf("%q not found\n", h)
					} else {
						return err
					}
				} else {
					fmt.Printf("%s: %v\n", h, err)
				}
			}
		}
	}

	pricing := map[string]float64{
		"email": 0.10 / 1000,
		"phone": 0.0130, // per minute
		"sms":   0.0075,
	}
	if false {
		dumpSortedMap("hosts", hosts)
	}
	fmt.Printf("has email: %v\n", hasEmail)
	fmt.Printf("has address: %v\n", hasAddress)
	dumpSortedMap("states", states)
	dumpSortedMap("preferred", preferred)
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
		resp, err := s3.New(session).GetObject(&s3.GetObjectInput{
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
		if _, err := s3.New(session).PutObject(&s3.PutObjectInput{
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
		r, err := d.Contact(ses.New(session), creds.NewClient())
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

func dump(i interface{}) {
	buf, _ := json.MarshalIndent(i, "", "  ")
	fmt.Println(string(buf))
}

func DevMode(c Config) error {
	sess, err := c.AWSSession()
	if err != nil {
		return fmt.Errorf("can't create session: %w", err)
	}
	creds, err := stw.LoadCredentials(s3.New(sess))
	if err != nil {
		return err
	}

	msg, err := jin.LoadMessage()
	if err != nil {
		return err
	}

	if true {
		resp, err := saws.SendEmail(
			ses.New(sess),
			"mra@xoba.com",
			"mra@xoba.com",
			jin.EmailSubject,
			msg,
		)
		if err != nil {
			return err
		}
		buf, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(buf))
	}
	if false {
		call, err := stw.MakeCall(
			creds.NewClient(),
			jin.TwilioNumber,
			"+19176086254",
			jin.TwimlURL,
		)
		if err != nil {
			return err
		}
		fmt.Println(call)
	}
	if false {

		resp, err := stw.SendSMS(
			creds.NewClient(),
			jin.TwilioNumber,
			"+19176086254",
			msg,
		)
		if err != nil {
			return err
		}
		buf, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(buf))
	}

	return nil
}
