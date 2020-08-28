package jin

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/kevinburke/twilio-go"
)

type ContactInfo struct {
	ID                  string    `json:",omitempty"`
	First, Middle, Last string    `json:",omitempty"`
	Preferred           Preferred `json:",omitempty"`
	Email               Addr      `json:",omitempty"`
	Mobile              Phone     `json:",omitempty"`
	Office              Phone     `json:",omitempty"`
	Home                Phone     `json:",omitempty"`
}

func (c ContactInfo) Decisions() ([]Decision, error) {
	var out []Decision
	add := func(d Decision) {
		out = append(out, d)
	}
	switch c.Preferred {
	case Email:
		add(NewEmail(c.Email))
	case Home:
		add(NewPhone(c.Home))
	case Mobile:
		add(NewPhone(c.Mobile))
	case Office:
		add(NewPhone(c.Office))
	case NoneSpecified:
		switch {
		case c.Mobile != "":
			add(NewPhone(c.Mobile))
			add(NewSMS(c.Mobile))
		case c.Home != "":
			add(NewPhone(c.Home))
		case c.Office != "":
			add(NewPhone(c.Office))
		}
	default:
		return nil, fmt.Errorf("unknown preferred for %s", c)
	}
	return out, nil
}

func (c ContactInfo) String() string {
	buf, _ := json.Marshal(c)
	return string(buf)
}

type Decision struct {
	Phone, Email, SMS *string `json:",omitempty"`
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

type Addr string
type Phone string

type Preferred string

const (
	Email         Preferred = "Email"
	Home          Preferred = "Home"
	Mobile        Preferred = "Mobile"
	Office        Preferred = "Office"
	NoneSpecified Preferred = "None"
)

// Notify Clients Function
func Notify(numbers, emails []string) error {
	twilioNumber := "+19083889127"
	sid := "ACad3070cb17d26d01a8fbdadb9cd7a37f"
	message := `Please note Dr.Qiu is leaving office and relocating in a few weeks. She will like you to find a new PCP in the next 30 days. Meantime if you want to make an appointment for urging matters and refills please do so ASAP. Also she has very limited hours 1 to 3 days a week. Please txt back or email or call if you desire to do so ASAP.
	Please let us know the new PCP Name, Phone #, Fax # that we will direct to Electronic Medical Records system to do so in the earliest possible time. You can find PCP info from your insurance company and Dr Qiu has a recommendation for Dr. Michelle Li (212)-688-8887 Please Contact her to see if she takes your insurance if not contact your insurance for a PCP who takes your insurance. We are only taking patients in July and August who need RX refills, Sick Visits, and Follow ups before Dr Qiu retirement. No GYN/Annual physicals.`
	twilioClient := twilio.NewClient(sid, "dc5ada9966d0b0d40ba1decc55da5bb7", nil)
	callURL, err := url.Parse("https://broyojo.com/message.mp3")
	if err != nil {
		return err
	}

	// loop through phone numbers, sending text messages and phone numbers
	for _, n := range numbers {
		if err := SendMessage(twilioClient, twilioNumber, n, message); err != nil {
			return err
		}
		MakeCall(twilioClient, twilioNumber, n, callURL)
	}
	return nil
}

// SendMessage Function
func SendMessage(client *twilio.Client, from, to, message string) error {
	_, err := client.Messages.SendMessage(from, to, message, nil)
	return err
}

// MakeCall Function
func MakeCall(client *twilio.Client, from, to string, callURL *url.URL) error {
	_, err := client.Calls.MakeCall(from, to, callURL)
	return err
}

// SendEmail Function
func SendEmail(from, to string, message string) error {
	return fmt.Errorf("SendEmail unimplemented")
}

// GetInfo of clients function
func LoadContacts(svc *s3.S3) ([]ContactInfo, error) {
	var contacts []ContactInfo

	lines, err := LoadCSV(svc, "patients.csv")
	if err != nil {
		return nil, err
	}

	var dups int
	ids := make(map[string]ContactInfo)
	headerIndex := make(map[string]int)
	for i, linex := range lines {
		if i == 0 {
			for j, f := range linex {
				headerIndex[f] = j
			}
			continue
		}
		field := func(name string) string {
			index, ok := headerIndex[name]
			if !ok {
				panic("no such name: " + name)
			}
			return strings.TrimSpace(linex[index])
		}
		if field("Fake") == "No" { // filter out fake clients (why does this exist?)
			contact := ContactInfo{
				ID:     field("Patient Identifier"),
				First:  field("Patient First Name"),
				Middle: field("Patient Middle Name"),
				Last:   field("Patient Last Name"),
				Email:  Addr(field("Email Address")),
			}
			set := func(value *Phone, name string) error {
				n, err := CleanNumber(field(name))
				if err != nil {
					return err
				}
				*value = Phone(n)
				return nil
			}

			if err := set(&contact.Home, "Home Phone"); err != nil {
				return nil, err
			}
			if err := set(&contact.Mobile, "Mobile Phone"); err != nil {
				return nil, err
			}
			if err := set(&contact.Office, "Office Phone"); err != nil {
				return nil, err
			}

			switch m := field("Preferred Method of Communication"); m {
			case "Mobile Phone":
				contact.Preferred = Mobile
			case "Email":
				contact.Preferred = Email
			case "Home Phone":
				contact.Preferred = Home
			case "Office Phone":
				contact.Preferred = Office
			case "":
				contact.Preferred = NoneSpecified
			default:
				return nil, fmt.Errorf("unhandled preferred: %q", m)
			}
			if _, ok := ids[contact.ID]; ok {
				dups++
			}
			ids[contact.ID] = contact
			contacts = append(contacts, contact)
		}
	}
	fmt.Printf("loaded %d dups, %d contacts total\n", dups, len(contacts))
	return contacts, nil
}

func LoadCSV(svc *s3.S3, filename string) ([][]string, error) {
	resp, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String("drjin"),
		Key:    aws.String("patients.csv"),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	lines, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		return nil, err
	}
	return lines, nil
}

func CleanNumber(number string) (string, error) {
	if number == "" {
		return "", nil
	}
	reg := regexp.MustCompile(`[^a-zA-Z0-9\+]+`)
	number = reg.ReplaceAllString(number, "")
	if !strings.HasPrefix(number, "+") {
		number = "+1" + number
	}
	if err := ValidateNumber(number); err != nil {
		return "", err
	}
	return number, nil
}

var e164 *regexp.Regexp = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

func ValidateNumber(phone string) error {
	if e164.MatchString(phone) {
		return nil
	}
	return fmt.Errorf("bad number: %q", phone)
}
