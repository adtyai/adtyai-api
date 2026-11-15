package common

import (
	"errors"
	"net"
	"net/netip"
	"net/smtp"
	"strings"

	ntlmssp "github.com/Azure/go-ntlmssp"
)

type smtpAutoAuth struct {
	username string
	password string
	mech     string
}

func AutoSMTPAuth(username, password string) smtp.Auth {
	return &smtpAutoAuth{username: username, password: password}
}

func (a *smtpAutoAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// net/smtp.PlainAuth refuses to send credentials over a connection that is
	// neither TLS nor localhost, because the advertised mechanisms cannot be
	// trusted otherwise. The custom LOGIN and NTLM mechanisms below must keep
	// that guard instead of asking the server for the password first.
	if err := checkSMTPAuthConnection(server); err != nil {
		return "", nil, err
	}
	useLoginAuth := SMTPForceAuthLogin
	if !useLoginAuth && shouldUseSMTPLoginAuth() {
		useLoginAuth = !(server != nil && len(server.Auth) == 1 && smtpServerSupportsAuth(server, "NTLM"))
	}
	if useLoginAuth {
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	}

	switch {
	case smtpServerSupportsAuth(server, "PLAIN"):
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.username, a.password, SMTPServer).Start(server)
	case smtpServerSupportsAuth(server, "LOGIN"):
		a.mech = "LOGIN"
		return "LOGIN", []byte{}, nil
	case smtpServerSupportsAuth(server, "NTLM"):
		a.mech = "NTLM"
		negotiateMessage, err := ntlmssp.NewNegotiateMessage("", "")
		if err != nil {
			return "", nil, err
		}
		return "NTLM", negotiateMessage, nil
	default:
		a.mech = "PLAIN"
		return smtp.PlainAuth("", a.username, a.password, SMTPServer).Start(server)
	}
}

func (a *smtpAutoAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	switch a.mech {
	case "LOGIN":
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown SMTP AUTH LOGIN challenge")
		}
	case "NTLM":
		return ntlmssp.NewAuthenticateMessage(fromServer, a.username, a.password, nil)
	default:
		return nil, errors.New("unexpected SMTP auth challenge")
	}
}

// checkSMTPAuthConnection mirrors the guard net/smtp.PlainAuth applies before
// handing out credentials.
func checkSMTPAuthConnection(server *smtp.ServerInfo) error {
	if server == nil || (!server.TLS && !isLocalSMTPHost(server.Name)) {
		return errors.New("unencrypted connection")
	}
	return nil
}

// isLocalSMTPHost mirrors net/smtp's localhost exemption: the bare name
// "localhost" (optionally with a port), any "*.localhost" name, or a loopback
// address.
func isLocalSMTPHost(name string) bool {
	host := name
	if splitHost, _, err := net.SplitHostPort(name); err == nil {
		host = splitHost
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil && addr.IsLoopback() {
		return true
	}
	return false
}

func smtpServerSupportsAuth(server *smtp.ServerInfo, mechanism string) bool {
	if server == nil {
		return false
	}
	for _, auth := range server.Auth {
		if strings.EqualFold(auth, mechanism) {
			return true
		}
	}
	return false
}
