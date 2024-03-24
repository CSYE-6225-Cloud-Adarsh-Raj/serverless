package serverless

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/functions/metadata"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	_ "github.com/lib/pq"
)

// PubSubMessage is the payload of a Pub/Sub event.
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// VerificationMessage represents the structure of the message expected from Pub/Sub.
type VerificationMessage struct {
	Email             string `json:"email"`
	VerificationToken string `json:"verificationToken"`
}

func getDatabaseURL() string {
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbName := os.Getenv("DB_NAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	sslMode := "disable"

	// Construct the connection string
	dbURL := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=%s password=%s", dbHost, dbUser, dbName, sslMode, dbPassword)

	return dbURL
}

func connectToDB() (*sql.DB, error) {
	dbURL := getDatabaseURL()
	return sql.Open("postgres", dbURL)
}

// SendVerificationEmail sends a verification email to the user.
func SendVerificationEmail(ctx context.Context, m PubSubMessage) error {
	meta, err := metadata.FromContext(ctx)
	if err != nil {
		log.Printf("metadata.FromContext: %v", err)
		return err
	}
	log.Printf("Function triggered by change to: %v", meta.Resource)

	log.Printf("Received data: %s", string(m.Data))

	data, err := base64.StdEncoding.DecodeString(string(m.Data))
	if err != nil {
		log.Printf("Assuming data is not base64 encoded due to error: %v", err)
		data = m.Data
	}

	var vMessage VerificationMessage
	err = json.Unmarshal(data, &vMessage)
	if err != nil {
		log.Printf("json.Unmarshal: %v", err)
		return err
	}

	return sendEmail(vMessage.Email, vMessage.VerificationToken)
}

// sendEmail sends an email using the SendGrid API.
func sendEmail(email, token string) error {
	// Connect to the database
	db, err := connectToDB()
	if err != nil {
		log.Printf("Failed to connect to database: %v", err)
		return err
	}
	defer db.Close()

	from := mail.NewEmail("Example User", "adarshrajneu@gmail.com")
	subject := "Verify Your Email Address"
	to := mail.NewEmail("Example User", email)
	plainTextContent := fmt.Sprintf("Please verify your email address by clicking on the link: %s", verificationURL(token))
	htmlContent := fmt.Sprintf("Please verify your email address by clicking on the link: <a href=\"%s\">%s</a>", verificationURL(token), verificationURL(token))
	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))
	response, err := client.Send(message)
	if err != nil {
		log.Printf("Failed to send email: %v", err)
		return err
	} else {
		log.Printf("Email sent: %v", response.StatusCode)
		_, err = db.Exec("INSERT INTO email_verification (email, uuid, time_sent) VALUES ($1, $2, $3)", email, token, time.Now())
		if err != nil {
			log.Printf("Failed to insert email verification record: %v", err)
			return err
		}

		log.Printf("Email verification record inserted successfully")
	}
	return nil
}

// verificationURL constructs the verification URL.
func verificationURL(token string) string {
	return fmt.Sprintf("https://rajadarsh.me/verify?token=%s", token)
}
