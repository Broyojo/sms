package jin

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type ContactInfo struct {
	ID                  string    `json:",omitempty"`
	First, Middle, Last string    `json:",omitempty"`
	Preferred           Preferred `json:",omitempty"`
	Email               Addr      `json:",omitempty"`
	Mobile              Phone     `json:",omitempty"`
	Office              Phone     `json:",omitempty"`
	Home                Phone     `json:",omitempty"`
	Address1, Address2  string    `json:",omitempty"`
	City, State, Zip    string    `json:",omitempty"`
}

func (c ContactInfo) Host() string {
	email := strings.TrimSpace(strings.ToLower(string(c.Email)))
	i := strings.LastIndexByte(email, '@')
	return email[i+1:]
}

var emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func (c ContactInfo) ValidateEmail() error {
	if !emailRegexp.MatchString(string(c.Email)) {
		return fmt.Errorf("bad email: %q", c.Email)
	}
	return nil
}

func (c ContactInfo) HasAddress() bool {
	if c.Address1 == "" {
		return false
	}
	if c.City == "" {
		return false
	}
	if c.State == "" {
		return false
	}
	return true
}

func (c ContactInfo) Decisions() ([]Decision, error) {
	var out []Decision
	add := func(d Decision) {
		if err := d.Validate(); err != nil {
			return
		}
		out = append(out, d)
	}
	none := func() {
		if c.Email != "" {
			add(NewEmail(c.Email))
		}
		switch {
		case c.Mobile != "":
			add(NewPhone(c.Mobile))
			add(NewSMS(c.Mobile))
		case c.Home != "":
			add(NewPhone(c.Home))
		case c.Office != "":
			add(NewPhone(c.Office))
		}
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
		none()
	default:
		return nil, fmt.Errorf("unknown preferred for %s", c)
	}
	for _, d := range out {
		if err := d.Validate(); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		none()
	}
	return out, nil
}

func (c ContactInfo) String() string {
	buf, _ := json.Marshal(c)
	return string(buf)
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
				//fmt.Printf("%2d: %q\n", j, f)
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
				ID:       field("Patient Identifier"),
				First:    field("Patient First Name"),
				Middle:   field("Patient Middle Name"),
				Last:     field("Patient Last Name"),
				Email:    Addr(field("Email Address")),
				Address1: field("Address Line 1"),
				Address2: field("Address Line 2"),
				City:     field("City"),
				State:    field("State"),
				Zip:      field("Postal Code"),
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
