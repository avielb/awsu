package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/kreuzwerker/awsu/config"
	"github.com/kreuzwerker/awsu/strategy"
	"github.com/pkg/errors"
)

// Console is a helper for opening links to the AWS console
type Console struct {
	conf    *config.Config
	profile *config.Profile
}

const (
	errFederationMarshal   = "failed to marshal federation session"
	errFederationRequest   = "failed to request federation"
	errFederationResponse  = "failed to receive federation response body"
	errFederationUnmarshal = "failed to unmarshal sign-in token"
	errNoSuchProfile       = "no such profile %q configured"
)

// New instantiates a new console helper
func New(conf *config.Config) (*Console, error) {

	profile, ok := conf.Profiles[conf.Profile]

	if !ok {
		return nil, fmt.Errorf(errNoSuchProfile, conf.Profile)
	}

	return &Console{
		conf:    conf,
		profile: profile,
	}, nil

}

// Link returns a link to the AWS console
func (c *Console) Link() (string, error) {

	var f = c.linkInternal

	if c.profile.ExternalID != "" {
		f = c.linkExternal
	}

	return f()

}

func (c *Console) linkInternal() (string, error) {

	a, err := arn.Parse(c.profile.RoleARN)

	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://signin.aws.amazon.com/switchrole?account=%s&roleName=%s&displayName=%s",
		a.AccountID,
		strings.TrimPrefix(a.Resource, "role/"),
		c.profile.Name)

	return url, nil

}

func (c *Console) linkExternal() (string, error) {

	creds, err := strategy.Apply(c.conf)

	if err != nil {
		return "", err
	}

	fep := map[string]string{
		"sessionId":    creds.AccessKeyID,
		"sessionKey":   creds.SessionToken,
		"sessionToken": creds.SessionToken,
	}

	enc, err := json.Marshal(fep)

	if err != nil {
		return "", errors.Wrapf(err, errFederationMarshal)
	}

	url := fmt.Sprintf("https://signin.aws.amazon.com/federation?Action=getSigninToken&Session=%s", string(url.QueryEscape(string(enc))))

	var buf = bytes.NewBuffer(nil)

	res, err := http.Get(url)

	if err != nil {
		return "", errors.Wrapf(err, errFederationRequest)
	}

	defer res.Body.Close()

	if _, err := io.Copy(buf, res.Body); err != nil {
		return "", errors.Wrapf(err, errFederationResponse)
	}

	var body map[string]string

	if err := json.Unmarshal(buf.Bytes(), &body); err != nil {
		return "", errors.Wrapf(err, errFederationUnmarshal)
	}

	var (
		destination = "https://console.aws.amazon.com/"
		issuer      = ""
		token       = body["SigninToken"]
	)

	url = fmt.Sprintf("https://signin.aws.amazon.com/federation?Action=login&Issuer=%s&Destination=%s&SigninToken=%s\n",
		issuer,
		destination,
		token)

	return url, nil

}