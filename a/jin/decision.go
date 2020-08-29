package jin

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
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

func (d *Decision) SetDebugging() {
	switch {
	case d.Phone != nil:
		d.Phone = aws.String("+19176086254")
	case d.SMS != nil:
		d.SMS = aws.String("+19176086254")
	case d.Email != nil:
		d.Email = aws.String("mra@xoba.com")
	}
}

func (d Decision) Key() string {
	h := md5.New()
	e := json.NewEncoder(h)
	e.Encode(d)
	return fmt.Sprintf("%x.json", h.Sum(nil))
}

type Receipt struct {
	Time       time.Time
	Successful bool
	Decision   Decision
	Content    interface{}
}

const (
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
		line := s.Text()
		fmt.Fprintln(w, line)
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return w.String(), nil
}

func (c Decision) Contact() (*Receipt, error) {
	r := Receipt{
		Time: time.Now(),
	}
	switch {
	case c.Phone != nil:

	case c.SMS != nil:

	case c.Email != nil:

	default:
		return nil, fmt.Errorf("no decision")
	}
	return &r, fmt.Errorf("unimplemented")
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
