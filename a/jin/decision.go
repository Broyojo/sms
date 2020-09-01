package jin

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/kevinburke/twilio-go"
	"github.com/xoba/sms/a/saws"
	"github.com/xoba/sms/a/stw"
)

func NewEmail(e Addr) Decision {
	s := string(e)
	return Decision{Email: &s}
}
func NewPhone(e Phone) Decision {
	s := string(e)
	return Decision{Phone: &s}
}
func NewSMS(e Phone) Decision {
	s := string(e)
	return Decision{SMS: &s}
}

type Decision struct {
	Phone, Email, SMS *string `json:",omitempty"`
}

// makes sure to not send for real
func (d *Decision) SetDebugging(phone, email string) {
	switch {
	case d.Phone != nil:
		d.Phone = aws.String(phone)
	case d.SMS != nil:
		d.SMS = aws.String(phone)
	case d.Email != nil:
		d.Email = aws.String(email)
	default:
		panic(fmt.Errorf("illegal decision: %v", d))
	}
}

func (d Decision) Key() string {
	h := md5.New()
	e := json.NewEncoder(h)
	e.Encode(d)
	return fmt.Sprintf("%x.json", h.Sum(nil))
}

const (
	EmailSender  = "dr2127588851@gmail.com"
	EmailSubject = "Important message from Dr. Ann Jin Qiu"
	TwilioNumber = "+19083889127"
	TwimlURL     = "https://broyojo.com/twilio"
)

func LoadMessage() (string, error) {
	w := new(bytes.Buffer)
	f, err := os.Open("message.txt")
	if err != nil {
		return "", err
	}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "%s ", line)
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	msg := w.String()
	r := regexp.MustCompile("[ ]+")
	msg = r.ReplaceAllString(msg, " ")
	return msg, nil
}

type Receipt struct {
	Time       time.Time
	Successful bool
	Decision   Decision
	Content    interface{}
}

func (r Receipt) String() string {
	buf, _ := json.MarshalIndent(r, "", "  ")
	return string(buf)
}

func (c Decision) Host() string {
	var host string
	if c.Email != nil {
		email := strings.TrimSpace(strings.ToLower(string(*c.Email)))
		i := strings.LastIndexByte(email, '@')
		host = email[i+1:]
	}
	return host
}

func IllegalNumber(n string) bool {
	if strings.HasPrefix(n, "+8") {
		return true
	}
	return false
}

func (c Decision) Contact(emailSvc *ses.SES, twilioSvc *twilio.Client) (*Receipt, error) {
	r := Receipt{
		Time:     time.Now(),
		Decision: c,
	}
	msg, err := LoadMessage()
	if err != nil {
		return nil, err
	}
	switch {
	case c.Phone != nil:
		if IllegalNumber(*c.Phone) {
			r.Content = "illegal number"
			// r.Successful == false
			return &r, nil
		}
		call, err := stw.MakeCall(
			twilioSvc,
			TwilioNumber,
			*c.Phone,
			TwimlURL,
		)
		if err != nil {
			return nil, err
		}
		r.Content = call
	case c.SMS != nil:
		if IllegalNumber(*c.SMS) {
			r.Content = "illegal number"
			// r.Successful == false
			return &r, nil
		}
		message, err := stw.SendSMS(
			twilioSvc,
			TwilioNumber,
			*c.SMS,
			msg,
		)
		if err != nil {
			return nil, err
		}
		r.Content = message
	case c.Email != nil:
		resp, err := saws.SendEmail(
			emailSvc,
			EmailSender,
			*c.Email,
			EmailSubject,
			msg,
		)
		if err != nil {
			return nil, err
		}
		r.Content = resp
	default:
		return nil, fmt.Errorf("no decision")
	}
	r.Successful = true
	return &r, nil
}

func (c Decision) Type() string {
	if c.Phone != nil {
		return "phone"
	}
	if c.Email != nil {
		return "email"
	}
	if c.SMS != nil {
		return "sms"
	}
	panic("illegal")
}

func (c Decision) String() string {
	buf, _ := json.Marshal(c)
	return string(buf)
}

func (d Decision) Validate() error {
	var nonNil int
	if d.Phone != nil {
		nonNil++
		if len(*d.Phone) == 0 {
			return fmt.Errorf("empty phone")
		}
	}
	if d.Email != nil {
		nonNil++
		if len(*d.Email) == 0 {
			return fmt.Errorf("empty email")
		}
	}
	if d.SMS != nil {
		nonNil++
		if len(*d.SMS) == 0 {
			return fmt.Errorf("empty sms")
		}
	}
	if nonNil == 1 {
		return nil
	}
	return fmt.Errorf("bad decision: " + d.String())
}
