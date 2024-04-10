package serverless

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/functions/metadata"
	_ "github.com/lib/pq"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/sirupsen/logrus"
)

// Initialize a logrus logger
var log = logrus.New()

func init() {
	// Set the log output format to JSON
	log.SetFormatter(&logrus.JSONFormatter{})
	// You can also set the Output to any `io.Writer` such as a file
	log.SetOutput(os.Stdout)
}

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

	dbURL := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=%s password=%s", dbHost, dbUser, dbName, sslMode, dbPassword)
	return dbURL
}

func connectToDB() (*sql.DB, error) {
	dbURL := getDatabaseURL()
	return sql.Open("postgres", dbURL)
}

func SendVerificationEmail(ctx context.Context, m PubSubMessage) error {
	meta, err := metadata.FromContext(ctx)
	if err != nil {
		log.WithError(err).Error("metadata.FromContext failed")
		return err
	}
	log.WithField("resource", meta.Resource).Info("Function triggered by change to resource")

	data, err := base64.StdEncoding.DecodeString(string(m.Data))
	if err != nil {
		log.WithError(err).Warn("Assuming data is not base64 encoded")
		data = m.Data
	} else {
		log.WithField("data", string(data)).Info("Received base64 encoded data")
	}

	var vMessage VerificationMessage
	err = json.Unmarshal(data, &vMessage)
	if err != nil {
		log.WithError(err).Error("json.Unmarshal failed")
		return err
	}

	return sendEmail(vMessage.Email, vMessage.VerificationToken)
}

func sendEmail(email, token string) error {
	db, err := connectToDB()
	if err != nil {
		log.WithError(err).Error("Failed to connect to database")
		return err
	}
	defer db.Close()

	from := mail.NewEmail("Admin@rajadarsh.me", "no-reply@rajadarsh.me")
	to := mail.NewEmail("Webapp User", email)
	templateID := os.Getenv("TEMPLATE_ID")

	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.SetTemplateID(templateID)

	p := mail.NewPersonalization()
	p.AddTos(to)
	p.SetDynamicTemplateData("verificationLink", verificationURL(token))
	p.SetDynamicTemplateData("contactLink", "mailto:support@rajadarsh.me")
	message.AddPersonalizations(p)

	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))
	response, err := client.Send(message)
	if err != nil {
		log.WithError(err).Error("Failed to send email")
		return err
	} else {
		log.WithFields(logrus.Fields{
			"statusCode": response.StatusCode,
			"body":       response.Body,
		}).Info("Email sent successfully")
		expiryTime := time.Now().Add(120 * time.Second)
		_, err = db.Exec("INSERT INTO email_verifications (email, uuid, expiry_time) VALUES ($1, $2, $3)", email, token, expiryTime)
		if err != nil {
			log.WithError(err).Error("Failed to insert email verification record with expiry time")
			return err
		}
	}

	log.Info("Email verification record inserted successfully")
	return nil
}

// func verificationURL(token string) string {
// 	// return fmt.Sprintf("http://rajadarsh.me:8080/verify?token=%s", token)
// 	return fmt.Sprintf("https://rajadarsh.me/verify?token=%s", token)
// }

func verificationURL(token string) string {
	baseUrl := os.Getenv("VERIFICATION_URL")
	// if baseUrl == "" {
	//     log.Warn("VERIFICATION_URL not set, using default")
	//     baseUrl = "https://rajadarsh.me/verify" // Default value if not set in env
	// }
	return fmt.Sprintf("%s?token=%s", baseUrl, token)
}
