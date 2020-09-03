package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/xoba/sms/a/jin"
	"github.com/xoba/sms/a/saws"
	"github.com/xoba/sms/a/stw"
	"github.com/xoba/sms/a/task"
	"golang.org/x/time/rate"
)

func main() {
	if err := Run(); err != nil {
		log.Fatal(err)
	}
}

type Config struct {
	Profile  string  `json:",omitempty"`
	Quantity int     `json:",omitempty"`
	Hertz    float64 `json:",omitempty"`
	Mode     string  // test, dev, prod, or logs, or count
	Prod     bool    `json:",omitempty"`
	Verbose  bool    `json:",omitempty"`
}

func (c Config) String() string {
	buf, _ := json.Marshal(c)
	return string(buf)
}

func (c Config) AWSSession() (*session.Session, error) {
	return saws.NewSessionFromProfile(c.Profile)
}

func Run() error {
	var config Config
	flag.BoolVar(&config.Verbose, "v", false, "whether to run verbosely or not")
	flag.StringVar(&config.Profile, "p", "", "aws iam profile to use, if any")
	flag.StringVar(&config.Mode, "m", "dev", "mode: test, dev, or prod, logs, or count")
	flag.IntVar(&config.Quantity, "q", 0, "max quantity of folks to reach out to")
	flag.Float64Var(&config.Hertz, "f", 1, "max frequency of contact, hertz")
	flag.Parse()

	var f func(Config) error
	switch config.Mode {
	case "test":
		config.Prod = false
		f = TestMode
	case "dev":
		config.Prod = false
		f = ContactPatients
	case "prod":
		config.Prod = true
		f = ContactPatients
	case "logs":
		config.Prod = false
		f = FindLogs
	case "count":
		f = CountReceipts
	default:
		return fmt.Errorf("illegal mode: %q", config.Mode)
	}
	log.Printf("running with config %s\n", config)
	return f(config)
}

func loadReceipts(c Config) (map[string]bool, error) {
	session, err := c.AWSSession()
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool)
	svc := s3.New(session)
	f := func(x *s3.ListObjectsV2Output, b bool) bool {
		for _, o := range x.Contents {
			m[path.Base(*o.Key)] = true
		}
		return true
	}
	i := &s3.ListObjectsV2Input{
		Bucket: aws.String("drjin"),
		Prefix: aws.String("receipts/"),
	}
	if err := svc.ListObjectsV2Pages(i, f); err != nil {
		return nil, err
	}
	return m, nil
}

func CountReceipts(c Config) error {
	m, err := loadReceipts(c)
	if err != nil {
		return err
	}
	fmt.Printf("%d receipts\n", len(m))
	return nil
}

func FindLogs(c Config) error {
	const errorSid = "SM8f7fbe3e0351431c8e6013164060d9db"
	session, err := c.AWSSession()
	if err != nil {
		return err
	}
	svc := s3.New(session)

	fetch := func(key string) (*jin.Receipt, error) {
		resp, err := svc.GetObject(&s3.GetObjectInput{
			Bucket: aws.String("drjin"),
			Key:    aws.String(key),
		})
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		var r jin.Receipt
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("can't unmarshal %s: %w", key, err)
		}
		return &r, nil
	}

	types := make(map[string]int)

	f := func(x *s3.ListObjectsV2Output, b bool) bool {
		for _, o := range x.Contents {
			fmt.Printf("got key %q\n", *o.Key)
			r, err := fetch(*o.Key)
			if err != nil {
				log.Fatalf("can't get %q: %v", *o.Key, err)
			}
			types[r.Decision.Type()]++
			m, ok := r.Content.(map[string]interface{})
			if ok {
				sid, ok := m["sid"].(string)
				if ok && sid == errorSid {
					fmt.Println(r)
				}
			}
		}
		return true
	}
	i := &s3.ListObjectsV2Input{
		Bucket: aws.String("drjin"),
		Prefix: aws.String("receipts/"),
	}
	if err := svc.ListObjectsV2Pages(i, f); err != nil {
		return err
	}
	fmt.Printf("types = %v\n", types)
	return nil
}

func WithinWorkingHours() bool {
	if h := time.Now().Hour(); h >= 9 && h <= 17 {
		return true
	}
	return false
}

func WaitForWorkingHours() {
	for {
		if WithinWorkingHours() {
			return
		}
		log.Println("waiting for working hours")
		time.Sleep(time.Minute)
	}
}

func ContactPatients(c Config) error {
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
		if !c.Verbose {
			return
		}
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
	var allDecisions []jin.Decision
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
		if c.Verbose {
			fmt.Println(i)
		}
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
		allDecisions = append(allDecisions, decisions...)
		if c.Verbose {
			for _, d := range decisions {
				fmt.Printf("  %s\n", d)
				actions[d.Type()]++
			}
		}
	}

	{
		badHosts := make(map[string]bool)
		lock := new(sync.Mutex)
		update := func(host string) {
			lock.Lock()
			defer lock.Unlock()
			badHosts[host] = true
		}

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

		var tasks []task.Task
		for h := range hosts {
			if h == "" {
				continue
			}
			h := h
			tasks = append(tasks, func() error {
				if c.Verbose {
					fmt.Printf("host %q\n", h)
				}
				_, err := net.LookupMX(h)
				if err != nil {
					if nerr, ok := err.(*net.DNSError); ok && nerr.IsNotFound {
						update(h)
						if c.Verbose {
							fmt.Printf("%q not found\n", h)
						}
					} else {
						return err
					}
				}
				return nil
			})
		}

		if errs := task.Run(tasks, 20); len(errs) > 0 {
			return fmt.Errorf("first error %w; %v", errs[0], errs)
		}

		var filtered []jin.Decision
		for _, d := range allDecisions {
			if badHosts[d.Host()] {
				fmt.Printf("skipping bad host in %s\n", d)
				continue
			}
			filtered = append(filtered, d)
		}
		allDecisions = filtered
	}

	if c.Verbose {
		const minutes = 3
		msg, err := jin.LoadMessage()
		if err != nil {
			return err
		}
		pricing := map[string]float64{
			"email": 0.10 / 1000,
			"phone": 0.0130 * minutes,                          // per minute
			"sms":   0.0075 * math.Ceil(float64(len(msg))/140), // sms segments
		}
		dumpSortedMap("hosts", hosts)
		fmt.Printf("has email: %v\n", hasEmail)
		fmt.Printf("has address: %v\n", hasAddress)
		dumpSortedMap("states", states)
		dumpSortedMap("preferred", preferred)
		fmt.Printf("%d no-decision contacts, %d total decisions; %v\n", noDecisions, len(allDecisions), actions)
		var total float64
		for k, v := range pricing {
			n := actions[k]
			c := v * float64(n)
			total += c
			fmt.Printf("cost for %d %s: $%.2f\n", n, k, c)
		}
		fmt.Printf("total estimated cost for %d decisions: $%.2f\n", len(allDecisions), total)
	}

	rand.Shuffle(len(allDecisions), func(i, j int) {
		allDecisions[i], allDecisions[j] = allDecisions[j], allDecisions[i]
	})

	uniqueDecisions := make(map[string]bool)
	for _, d := range allDecisions {
		uniqueDecisions[d.Key()] = true
	}

	fmt.Printf("%d / %d unique decisions\n", len(uniqueDecisions), len(allDecisions))

	dt := time.Duration(1 / c.Hertz * float64(time.Second))
	fmt.Printf("running with limit dt = %v\n", dt)
	limiter := rate.NewLimiter(rate.Every(dt), 1)

	receipts, err := loadReceipts(c)
	if err != nil {
		return err
	}
	fmt.Printf("there are %d receipts\n", len(receipts))
	availableContacts := len(allDecisions) - len(receipts)
	fmt.Printf("approximately %d decisions to go\n", availableContacts)
	if availableContacts > c.Quantity {
		availableContacts = c.Quantity
	}

	var contactsMade int
	for _, d := range allDecisions {
		if contactsMade >= c.Quantity {
			break
		}
		if c.Prod {
			WaitForWorkingHours()
		}
		if !c.Prod {
			d.SetDebugging("+19176086254", "mra@xoba.com")
			if d.SMS == nil {
				continue
			}
			log.Printf("--> updated decision: %s", d)
		}
		if receipts[d.Key()] {
			continue
		}
		fmt.Println()
		log.Printf("%d/%d. decision: %s", 1+contactsMade, availableContacts, d)
		done, err := alreadyDone(session, d)
		if err != nil {
			return err
		}
		if done {
			log.Printf("already done: %s", d)
			continue
		}
		r, err := d.Contact(ses.New(session), creds.NewClient())
		if err != nil {
			return err
		}
		if err := markDone(session, r); err != nil {
			return err
		}
		contactsMade++
		log.Printf("finished and marked %s\n", d)
		if err := limiter.Wait(context.Background()); err != nil {
			return err
		}
	}

	return nil
}

func alreadyDone(session *session.Session, d jin.Decision) (bool, error) {
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
	if err := dec.Decode(&r); err != nil {
		return false, fmt.Errorf("can't unmarshal %s: %w", d.Key(), err)
	}
	return true, nil
}

func markDone(session *session.Session, r *jin.Receipt) error {
	buf, err := json.MarshalIndent(r, "", "  ")
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

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func dump(i interface{}) {
	buf, _ := json.MarshalIndent(i, "", "  ")
	fmt.Println(string(buf))
}

func TestMode(c Config) error {
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
