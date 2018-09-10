package credentials

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/kreuzwerker/awsu/log"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/yawn/envmap"
)

type Credentials struct {
	credentials.Value
	Expires time.Time
	Profile string
}

func NewLongTerm(profile, accessKeyId, secretAccessKey string) *Credentials {

	return &Credentials{
		Value: credentials.Value{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
		},
		Profile: profile,
	}

}

func NewShortTerm(profile, accessKeyId, secretAccessKey, sessionToken string, expires time.Time) *Credentials {

	return &Credentials{
		Value: credentials.Value{
			AccessKeyID:     accessKeyId,
			SecretAccessKey: secretAccessKey,
			SessionToken:    sessionToken,
		},
		Expires: expires,
		Profile: profile,
	}

}

// Load loads cached credentials
func Load(profile string) (*Credentials, error) {

	path, err := cachePath(profile)

	if err != nil {
		return nil, err
	}

	log.Debug("loading cached credentials from %q", path)

	raw, err := ioutil.ReadFile(path)

	if err != nil {
		return nil, err
	}

	var creds *Credentials

	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, err
	}

	if !creds.IsValid() {
		return nil, fmt.Errorf("existing credentials are invalid")
	}

	return creds, nil

}

// Exec sets an appropriate runtime environment and execs the passed in app
func (c *Credentials) Exec(app string, args []string) error {

	env := envmap.Import()

	if c.Expires.Second() > 0 {
		env["AWSU_EXPIRES"] = c.Expires.Format(time.RFC3339)
	}

	env["AWSU_PROFILE"] = c.Profile
	env["AWS_ACCESS_KEY_ID"] = c.Value.AccessKeyID
	env["AWS_SECRET_ACCESS_KEY"] = c.Value.SecretAccessKey
	env["AWS_SESSION_TOKEN"] = c.Value.SessionToken

	cmd, err := exec.LookPath(app)

	if err != nil {
		return err
	}

	log.Debug("running %q with args %q", cmd, args)

	return syscall.Exec(cmd, args, env.ToEnv())

}

func (c *Credentials) IsValid() bool {
	return time.Now().Before(c.Expires)
}

// Save caches credentials
func (c *Credentials) Save() error {

	path, err := cachePath(c.Profile)

	if err != nil {
		return err
	}

	log.Debug("saving session to %q", path)

	out, err := json.Marshal(c)

	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, out, 0600)

}

// String returns a string representation of these credentials, suitable for eval()
func (c *Credentials) String() string {

	parts := []string{
		fmt.Sprintf("export AWSU_EXPIRES=%s", c.Expires.Format(time.RFC3339)),
		fmt.Sprintf("export AWS_ACCESS_KEY_ID=%s", c.AccessKeyID),
		fmt.Sprintf("export AWS_SECRET_ACCESS_KEY=%s", c.SecretAccessKey),
	}

	if c.SessionToken != "" {
		parts = append(parts, fmt.Sprintf("export AWS_SESSION_TOKEN=%s", c.SessionToken))
	}

	return strings.Join(parts, "\n")

}

func (c *Credentials) NewSession() *session.Session {

	sess := session.New(&aws.Config{})

	return c.UpdateSession(sess)

}

func (c *Credentials) UpdateSession(sess *session.Session) *session.Session {

	sess.Config.Credentials = credentials.NewStaticCredentials(
		c.AccessKeyID,
		c.SecretAccessKey,
		c.SessionToken,
	)

	return sess

}

// cachePath determines the cache path for the given credentials
func cachePath(profile string) (string, error) {

	home, err := homedir.Dir()

	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".awsu", "sessions")

	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(dir, fmt.Sprintf("%s.json", profile)), nil

}
