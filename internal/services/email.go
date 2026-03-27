package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type EmailProvider string

const (
	ProviderSMTP     EmailProvider = "smtp"
	ProviderSendGrid EmailProvider = "sendgrid"
	ProviderMailgun  EmailProvider = "mailgun"
)

type EmailConfig struct {
	Provider       EmailProvider
	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPPassword   string
	FromName       string
	FromEmail      string
	UseTLS         bool
	SendGridAPIKey string
	MailgunDomain  string
	MailgunAPIKey  string
}

type EmailService struct {
	config           *EmailConfig
	client           *smtp.Client
	conn             net.Conn
	tlsConfig        *tls.Config
	mu               sync.Mutex
	enabled          bool
	templates        map[string]*template.Template
	templateSubjects map[string]string
}

type EmailRecipient struct {
	To   string
	Name string
}

type EmailData map[string]interface{}

type EmailTemplate struct {
	Subject  string
	HTMLBody string
	TextBody string
}

func NewEmailService(config *EmailConfig) *EmailService {
	es := &EmailService{
		config:           config,
		enabled:          config != nil && config.Provider != "",
		templates:        make(map[string]*template.Template),
		templateSubjects: make(map[string]string),
	}

	if es.enabled {
		es.loadDefaultTemplates()
	}

	return es
}

func (es *EmailService) loadDefaultTemplates() {
	templates := map[string]EmailTemplate{
		"welcome": {
			Subject:  "Welcome to Enclavr!",
			HTMLBody: welcomeHTML,
			TextBody: welcomeText,
		},
		"password_reset": {
			Subject:  "Reset Your Password",
			HTMLBody: passwordResetHTML,
			TextBody: passwordResetText,
		},
		"email_verification": {
			Subject:  "Verify Your Email",
			HTMLBody: emailVerificationHTML,
			TextBody: emailVerificationText,
		},
		"room_invite": {
			Subject:  "You've Been Invited!",
			HTMLBody: roomInviteHTML,
			TextBody: roomInviteText,
		},
		"password_changed": {
			Subject:  "Your Password Has Been Changed",
			HTMLBody: passwordChangedHTML,
			TextBody: passwordChangedText,
		},
		"two_factor_code": {
			Subject:  "Your Two-Factor Authentication Code",
			HTMLBody: twoFactorHTML,
			TextBody: twoFactorText,
		},
		"new_device_login": {
			Subject:  "New Device Login Detected",
			HTMLBody: newDeviceLoginHTML,
			TextBody: newDeviceLoginText,
		},
	}

	for name, tmpl := range templates {
		t, err := template.New(name).Funcs(template.FuncMap{
			"nowFormat": nowFormat,
		}).Parse(tmpl.HTMLBody)
		if err != nil {
			log.Printf("Error parsing template %s: %v", name, err)
			continue
		}
		es.templates[name] = t
		es.templateSubjects[name] = tmpl.Subject
	}
}

func (es *EmailService) Connect() error {
	if !es.enabled || es.config.Provider != ProviderSMTP {
		return nil
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	return es.connectLocked()
}

func (es *EmailService) connectLocked() error {
	var err error
	es.conn, err = net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", es.config.SMTPHost, es.config.SMTPPort),
		10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	es.client, err = smtp.NewClient(es.conn, es.config.SMTPHost)
	if err != nil {
		es.conn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}

	if es.config.UseTLS {
		es.tlsConfig = &tls.Config{
			ServerName:         es.config.SMTPHost,
			InsecureSkipVerify: false,
		}
		if err = es.client.StartTLS(es.tlsConfig); err != nil {
			if quitErr := es.client.Quit(); quitErr != nil {
				log.Printf("SMTP quit error: %v", quitErr)
			}
			es.conn.Close()
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	auth := smtp.PlainAuth("", es.config.SMTPHost, es.config.SMTPPassword, es.config.SMTPHost)
	if err = es.client.Auth(auth); err != nil {
		if quitErr := es.client.Quit(); quitErr != nil {
			log.Printf("SMTP quit error: %v", quitErr)
		}
		es.conn.Close()
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	return nil
}

func (es *EmailService) Disconnect() {
	if es.client != nil {
		if err := es.client.Quit(); err != nil {
			log.Printf("SMTP disconnect error: %v", err)
		}
	}
	if es.conn != nil {
		es.conn.Close()
	}
}

func (es *EmailService) Send(ctx context.Context, to EmailRecipient, subject, htmlBody, textBody string) error {
	if !es.enabled {
		log.Printf("Email service disabled, skipping send to %s", to.To)
		return nil
	}

	switch es.config.Provider {
	case ProviderSMTP:
		return es.sendSMTP(to, subject, htmlBody, textBody)
	case ProviderSendGrid:
		return es.sendSendGrid(to, subject, htmlBody, textBody)
	case ProviderMailgun:
		return es.sendMailgun(to, subject, htmlBody, textBody)
	default:
		return fmt.Errorf("unknown email provider: %s", es.config.Provider)
	}
}

func (es *EmailService) sendSMTP(to EmailRecipient, subject, htmlBody, textBody string) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.client == nil {
		if err := es.connectLocked(); err != nil {
			return err
		}
	}

	from := mail.Address{Name: es.config.FromName, Address: es.config.FromEmail}
	toAddr := mail.Address{Name: to.Name, Address: to.To}

	msg := buildMessage(from, toAddr, subject, htmlBody, textBody)

	err := es.client.Mail(from.Address)
	if err != nil {
		return fmt.Errorf("SMTP mail from failed: %w", err)
	}

	err = es.client.Rcpt(toAddr.Address)
	if err != nil {
		return fmt.Errorf("SMTP rcpt to failed: %w", err)
	}

	w, err := es.client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data failed: %w", err)
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}

	return nil
}

func (es *EmailService) sendSendGrid(to EmailRecipient, subject, htmlBody, textBody string) error {
	return fmt.Errorf("SendGrid provider not implemented")
}

func (es *EmailService) sendMailgun(to EmailRecipient, subject, htmlBody, textBody string) error {
	return fmt.Errorf("Mailgun provider not implemented")
}

func buildMessage(from, to mail.Address, subject, htmlBody, textBody string) string {
	var buf bytes.Buffer

	buf.WriteString("From: " + from.String() + "\r\n")
	buf.WriteString("To: " + to.String() + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=boundary\r\n\r\n")

	buf.WriteString("--boundary\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(textBody)
	buf.WriteString("\r\n\r\n")

	buf.WriteString("--boundary\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody)
	buf.WriteString("\r\n\r\n")

	buf.WriteString("--boundary--\r\n")

	return buf.String()
}

func (es *EmailService) SendTemplate(ctx context.Context, to EmailRecipient, templateName string, data EmailData) error {
	tmpl, ok := es.templates[templateName]
	if !ok {
		return fmt.Errorf("template not found: %s", templateName)
	}

	var htmlBuf bytes.Buffer
	if err := tmpl.Execute(&htmlBuf, data); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	textBuf := bytes.Buffer{}
	textBuf.WriteString(es.plainText(htmlBuf.String()))

	subject := es.templateSubjects[templateName]
	return es.Send(ctx, to, subject, htmlBuf.String(), textBuf.String())
}

func (es *EmailService) plainText(html string) string {
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "</p>", "\n\n")
	html = strings.ReplaceAll(html, "</div>", "\n")
	html = strings.ReplaceAll(html, "<ul>", "\n")
	html = strings.ReplaceAll(html, "</ul>", "\n")
	html = strings.ReplaceAll(html, "<li>", "  - ")
	html = strings.ReplaceAll(html, "</li>", "\n")

	text := stripTags(html)
	text = strings.TrimSpace(text)
	return text
}

func stripTags(html string) string {
	var buf bytes.Buffer
	inTag := false

	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			buf.WriteRune(r)
		}
	}

	return buf.String()
}

func (es *EmailService) SendWelcomeEmail(ctx context.Context, to EmailRecipient, verifyURL string) error {
	return es.SendTemplate(ctx, to, "welcome", EmailData{
		"Name":      to.Name,
		"VerifyURL": verifyURL,
	})
}

func (es *EmailService) SendPasswordResetEmail(ctx context.Context, to EmailRecipient, resetToken string) error {
	return es.SendTemplate(ctx, to, "password_reset", EmailData{
		"Name":       to.Name,
		"ResetToken": resetToken,
	})
}

func (es *EmailService) SendEmailVerificationEmail(ctx context.Context, to EmailRecipient, verifyToken string) error {
	return es.SendTemplate(ctx, to, "email_verification", EmailData{
		"Name":        to.Name,
		"VerifyToken": verifyToken,
	})
}

func (es *EmailService) SendRoomInviteEmail(ctx context.Context, to EmailRecipient, roomName, inviterName, inviteURL string) error {
	return es.SendTemplate(ctx, to, "room_invite", EmailData{
		"Name":      to.Name,
		"RoomName":  roomName,
		"Inviter":   inviterName,
		"InviteURL": inviteURL,
	})
}

func (es *EmailService) SendPasswordChangedEmail(ctx context.Context, to EmailRecipient) error {
	return es.SendTemplate(ctx, to, "password_changed", EmailData{
		"Name": to.Name,
	})
}

func (es *EmailService) SendTwoFactorCodeEmail(ctx context.Context, to EmailRecipient, code string) error {
	return es.SendTemplate(ctx, to, "two_factor_code", EmailData{
		"Name": to.Name,
		"Code": code,
	})
}

func (es *EmailService) SendNewDeviceLoginEmail(ctx context.Context, to EmailRecipient, device, ip, location, loginTime string) error {
	return es.SendTemplate(ctx, to, "new_device_login", EmailData{
		"Name":     to.Name,
		"Device":   device,
		"IP":       ip,
		"Location": location,
		"Time":     loginTime,
	})
}

func (es *EmailService) IsEnabled() bool {
	return es.enabled
}

type templateInfo struct {
	*template.Template
	Subject string
}

func (es *EmailService) ParseTemplate(name string) (*templateInfo, error) {
	t, ok := es.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %s not found", name)
	}

	subject := ""
	switch name {
	case "welcome":
		subject = "Welcome to Enclavr!"
	case "password_reset":
		subject = "Reset Your Password"
	case "email_verification":
		subject = "Verify Your Email"
	case "room_invite":
		subject = "You've Been Invited!"
	case "password_changed":
		subject = "Your Password Has Been Changed"
	case "two_factor_code":
		subject = "Your Two-Factor Authentication Code"
	case "new_device_login":
		subject = "New Device Login Detected"
	}

	return &templateInfo{t, subject}, nil
}

var welcomeHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome to Enclavr</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="text-align: center; margin-bottom: 30px;">
        <h1 style="color: #6366f1; margin: 0;">Enclavr</h1>
    </div>
    
    <h2>Welcome, {{.Name}}!</h2>
    
    <p>Thank you for joining Enclavr! We're excited to have you on our platform.</p>
    
    <p>To get started, please verify your email address by clicking the button below:</p>
    
    <div style="text-align: center; margin: 30px 0;">
        <a href="{{.VerifyURL}}" style="background-color: #6366f1; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">Verify Email</a>
    </div>
    
    <p>If the button doesn't work, you can copy and paste this link into your browser:</p>
    <p style="word-break: break-all; color: #666;">{{.VerifyURL}}</p>
    
    <p>If you didn't create an account, you can safely ignore this email.</p>
    
    <hr style="border: none; border-top: 1px solid #eee; margin: 30px 0;">
    
    <p style="color: #888; font-size: 12px;">
        &copy; {{nowFormat "2006"}} Enclavr. All rights reserved.
    </p>
</body>
</html>`

var welcomeText = `Welcome to Enclavr!

Hi {{.Name}},

Thank you for joining Enclavr! We're excited to have you on our platform.

To get started, please verify your email address by visiting this link:
{{.VerifyURL}}

If you didn't create an account, you can safely ignore this email.

&copy; {{nowFormat "2006"}} Enclavr. All rights reserved.`

var passwordResetHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Reset Your Password</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>Reset Your Password</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p>We received a request to reset your password. Use the token below to create a new password:</p>
    
    <div style="background-color: #f5f5f5; padding: 15px; border-radius: 6px; text-align: center; font-family: monospace; font-size: 18px; letter-spacing: 2px; margin: 20px 0;">
        {{.ResetToken}}
    </div>
    
    <p>This token will expire in 1 hour.</p>
    
    <p>If you didn't request a password reset, please ignore this email or contact support if you have concerns.</p>
</body>
</html>`

var passwordResetText = `Reset Your Password

Hi {{.Name}},

We received a request to reset your password. Use the token below:
{{.ResetToken}}

This token will expire in 1 hour.

If you didn't request a password reset, please ignore this email.`

var emailVerificationHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Verify Your Email</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>Verify Your Email</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p>Please verify your email address using the token below:</p>
    
    <div style="background-color: #f5f5f5; padding: 15px; border-radius: 6px; text-align: center; font-family: monospace; font-size: 18px; letter-spacing: 2px; margin: 20px 0;">
        {{.VerifyToken}}
    </div>
    
    <p>This token will expire in 24 hours.</p>
</body>
</html>`

var emailVerificationText = `Verify Your Email

Hi {{.Name}},

Please verify your email address using the token below:
{{.VerifyToken}}

This token will expire in 24 hours.`

var roomInviteHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>You've Been Invited!</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>You've Been Invited!</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p><strong>{{.Inviter}}</strong> has invited you to join <strong>{{.RoomName}}</strong> on Enclavr!</p>
    
    <div style="text-align: center; margin: 30px 0;">
        <a href="{{.InviteURL}}" style="background-color: #6366f1; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; display: inline-block;">Join Room</a>
    </div>
    
    <p>If the button doesn't work, you can copy and paste this link:</p>
    <p style="word-break: break-all; color: #666;">{{.InviteURL}}</p>
</body>
</html>`

var roomInviteText = `You've Been Invited!

Hi {{.Name}},

{{.Inviter}} has invited you to join {{.RoomName}} on Enclavr!

Join using this link:
{{.InviteURL}}`

var passwordChangedHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Password Changed</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>Password Changed</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p>Your password has been successfully changed.</p>
    
    <p>If you didn't change your password, please contact our support team immediately.</p>
</body>
</html>`

var passwordChangedText = `Password Changed

Hi {{.Name}},

Your password has been successfully changed.

If you didn't change your password, please contact our support team immediately.`

var twoFactorHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Two-Factor Authentication Code</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>Your Authentication Code</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p>Your two-factor authentication code is:</p>
    
    <div style="background-color: #f5f5f5; padding: 20px; border-radius: 6px; text-align: center; font-family: monospace; font-size: 32px; letter-spacing: 8px; margin: 20px 0; font-weight: bold;">
        {{.Code}}
    </div>
    
    <p>This code will expire in 5 minutes.</p>
    
    <p>If you didn't request this code, please ignore this email.</p>
</body>
</html>`

var twoFactorText = `Your Two-Factor Authentication Code

Hi {{.Name}},

Your authentication code is: {{.Code}}

This code will expire in 5 minutes.

If you didn't request this code, please ignore this email.`

var newDeviceLoginHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>New Device Login</title>
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
    <h2>New Device Login</h2>
    
    <p>Hi {{.Name}},</p>
    
    <p>We detected a new login to your Enclavr account.</p>
    
    <div style="background-color: #f5f5f5; padding: 15px; border-radius: 6px; margin: 20px 0;">
        <p style="margin: 5px 0;"><strong>Device:</strong> {{.Device}}</p>
        <p style="margin: 5px 0;"><strong>IP Address:</strong> {{.IP}}</p>
        <p style="margin: 5px 0;"><strong>Location:</strong> {{.Location}}</p>
        <p style="margin: 5px 0;"><strong>Time:</strong> {{.Time}}</p>
    </div>
    
    <p>If this was you, you can ignore this email.</p>
    
    <p>If you didn't log in, please:</p>
    <ul>
        <li>Change your password immediately</li>
        <li>Enable two-factor authentication</li>
        <li>Review your account settings</li>
    </ul>
</body>
</html>`

var newDeviceLoginText = `New Device Login

Hi {{.Name}},

We detected a new login to your Enclavr account.

Device: {{.Device}}
IP Address: {{.IP}}
Location: {{.Location}}
Time: {{.Time}}

If this was you, you can ignore this email.

If you didn't log in, please:
- Change your password immediately
- Enable two-factor authentication
- Review your account settings`

func nowFormat(format string) string {
	return time.Now().Format(format)
}

var _ = nowFormat // TODO: use in templates
