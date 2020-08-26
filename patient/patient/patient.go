package patient

import (
	"encoding/csv"
	"net/url"
	"os"
	"regexp"

	"github.com/kevinburke/twilio-go"
)

type ContactInfo struct {
	Email, Phone, SMS string
}

// Notify Clients Function
func Notify(numbers, emails []string) {
	twilioNumber := "+19083889127"
	sid := "ACad3070cb17d26d01a8fbdadb9cd7a37f"
	message := `Please note Dr.Qiu is leaving office and relocating in a few weeks. She will like you to find a new PCP in the next 30 days. Meantime if you want to make an appointment for urging matters and refills please do so ASAP. Also she has very limited hours 1 to 3 days a week. Please txt back or email or call if you desire to do so ASAP.
	Please let us know the new PCP Name, Phone #, Fax # that we will direct to Electronic Medical Records system to do so in the earliest possible time. You can find PCP info from your insurance company and Dr Qiu has a recommendation for Dr. Michelle Li (212)-688-8887 Please Contact her to see if she takes your insurance if not contact your insurance for a PCP who takes your insurance. We are only taking patients in July and August who need RX refills, Sick Visits, and Follow ups before Dr Qiu retirement. No GYN/Annual physicals.`
	twilioClient := twilio.NewClient(sid, "dc5ada9966d0b0d40ba1decc55da5bb7", nil)
	callURL, err := url.Parse("https://broyojo.com/message.mp3")
	check(err)

	// loop through phone numbers, sending text messages and phone numbers
	for _, n := range numbers {
		SendMessage(twilioClient, twilioNumber, n, message)
		MakeCall(twilioClient, twilioNumber, n, callURL)
	}

}

// SendMessage Function
func SendMessage(client *twilio.Client, from, to, message string) {
	_, err := client.Messages.SendMessage(from, to, message, nil)
	check(err)
}

// MakeCall Function
func MakeCall(client *twilio.Client, from, to string, callURL *url.URL) {
	_, err := client.Calls.MakeCall(from, to, callURL)
	check(err)
}

// SendEmail Function
func SendEmail(from, to string, message string) {

}

// GetInfo of clients function
func GetInfo() []ContactInfo {
	contacts := []ContactInfo{}

	lines, err := ReadCsv("patients.csv")
	check(err)

	for _, line := range lines {
		if line[19] == "No" { // filter out fake clients (why does this exist?)
			contact := ContactInfo{}
			home := Clean(line[14])
			mobile := Clean(line[15])
			office := Clean(line[16])
			email := line[17]

			switch line[13] { // switch based on prefered contact method
			case "Mobile Phone":
				contact.Phone = mobile
				contact.SMS = mobile
			case "Email":
				contact.Email = email
			case "Home Phone":
				contact.Phone = home
			case "Office Phone":
				contact.Phone = office
			case "": //otherwise send on email, sms, and message
				contact.Email = email
				if mobile != "" {
					contact.Phone = mobile
					contact.SMS = mobile
				} else if home != "" {
					contact.Phone = home
				} else {
					contact.Phone = office
				}
			}
			contacts = append(contacts, contact)
		}
	}
	return contacts
}

func ReadCsv(filename string) ([][]string, error) {
	// Open CSV file
	f, err := os.Open(filename)
	if err != nil {
		return [][]string{}, err
	}
	defer f.Close()

	// Read File into a Variable
	lines, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return [][]string{}, err
	}
	return lines, nil
}

func Clean(number string) string {
	if number == "" {
		return ""
	}
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	check(err)
	number = reg.ReplaceAllString(number, "")
	number = "+1" + number
	return number
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
