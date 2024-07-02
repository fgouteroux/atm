// Copyright 2018 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-openapi/strfmt"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/matchers/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func username() string {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	return user.Username
}

type silenceAddCmd struct {
	author           string
	requireComment   bool
	duration         string
	maxDuration      string
	start            string
	end              string
	comment          string
	matchers         []string
	tenant           string
	tenantFile       string
	tenantHTTPHeader string
}

const silenceAddHelp = `Add a new alertmanager silence

  atm uses a simplified Prometheus syntax to represent silences. The
  non-option section of arguments constructs a list of "Matcher Groups"
  that will be used to create a number of silences. The following examples
  will attempt to show this behaviour in action:

  atm silence add alertname=foo node=bar

	This statement will add a silence that matches alerts with the
	alertname=foo and node=bar label value pairs set.

  atm silence add foo node=bar

	If alertname is omitted and the first argument does not contain a '=' or a
	'=~' then it will be assumed to be the value of the alertname pair.

  atm silence add 'alertname=~foo.*'

	As well as direct equality, regex matching is also supported. The '=~' syntax
	(similar to Prometheus) is used to represent a regex match. Regex matching
	can be used in combination with a direct match.
`

func configureSilenceAddCmd(cc *kingpin.CmdClause) {
	var (
		c      = &silenceAddCmd{}
		addCmd = cc.Command("add", silenceAddHelp)
	)
	addCmd.Flag("tenant", "tenant").Short('t').StringVar(&c.tenant)
	addCmd.Flag("tenant.http-header", "tenant HTTP Header").Default("X-Scope-OrgID").StringVar(&c.tenantHTTPHeader)
	addCmd.Flag("tenant.file", "tenant file location").PlaceHolder("<filename>").ExistingFileVar(&c.tenantFile)
	addCmd.Flag("author", "Username for CreatedBy field").Short('a').Default(username()).StringVar(&c.author)
	addCmd.Flag("require-comment", "Require comment to be set").Hidden().Default("true").BoolVar(&c.requireComment)
	addCmd.Flag("duration", "Duration of silence").Short('d').Default("1h").StringVar(&c.duration)
	addCmd.Flag("max-duration", "Max Duration of silence").Default("12h").StringVar(&c.maxDuration)
	addCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.start)
	addCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.end)
	addCmd.Flag("comment", "A comment to help describe the silence").Short('c').StringVar(&c.comment)
	addCmd.Arg("matcher-groups", "Query filter").StringsVar(&c.matchers)
	addCmd.Action(execWithTimeout(c.add))
}

func (c *silenceAddCmd) add(ctx context.Context, _ *kingpin.ParseContext) error {
	var err error

	if len(c.matchers) > 0 {
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets
		// assume that the user wants alertname=<arg> and prepend `alertname=`
		// to the front.
		_, err := compat.Matcher(c.matchers[0], "cli")
		if err != nil {
			c.matchers[0] = fmt.Sprintf("alertname=%s", strconv.Quote(c.matchers[0]))
		}
	}

	matchers := make([]labels.Matcher, 0, len(c.matchers))
	for _, s := range c.matchers {
		m, err := compat.Matcher(s, "cli")
		if err != nil {
			return err
		}
		matchers = append(matchers, *m)
	}
	if len(matchers) < 1 {
		return fmt.Errorf("no matchers specified")
	}

	var startsAt time.Time
	if c.start != "" {
		startsAt, err = time.Parse(time.RFC3339, c.start)
		if err != nil {
			return err
		}

	} else {
		startsAt = time.Now().UTC()
	}

	var endsAt time.Time
	if c.end != "" {
		endsAt, err = time.Parse(time.RFC3339, c.end)
		if err != nil {
			return err
		}
	} else {
		d, err := model.ParseDuration(c.duration)
		if err != nil {
			return err
		}
		if d == 0 {
			return fmt.Errorf("silence duration must be greater than 0")
		}
		endsAt = startsAt.UTC().Add(time.Duration(d))

		md, _ := model.ParseDuration(c.maxDuration)
		if d > md {
			return fmt.Errorf("silence duration '%s' couldn't be greater than '%s'", c.duration, c.maxDuration)
		}
	}

	if startsAt.After(endsAt) {
		return errors.New("silence cannot start after it ends")
	}

	if c.requireComment && c.comment == "" {
		return errors.New("comment required by config")
	}

	start := strfmt.DateTime(startsAt)
	end := strfmt.DateTime(endsAt)
	ps := &models.PostableSilence{
		Silence: models.Silence{
			Matchers:  TypeMatchers(matchers),
			StartsAt:  &start,
			EndsAt:    &end,
			CreatedBy: &c.author,
			Comment:   &c.comment,
		},
	}
	silenceParams := silence.NewPostSilencesParams().WithContext(ctx).WithSilence(ps)

	if c.tenant != "" && c.tenantFile != "" {
		kingpin.Fatalf("tenant and tenant.file are mutually exclusive")
	}

	httpConfig := NewAlertmanagerClientConfig()
	if c.tenant != "" {
		httpConfig = setHTTPTenantHeader(httpConfig, c.tenant, c.tenantHTTPHeader)
		amclient := NewAlertmanagerClient(alertmanagerURL, *httpConfig)

		postOk, err := amclient.Silence.PostSilences(silenceParams)
		if err != nil {
			return fmt.Errorf("Unable to add silence for '%s' tenant: %v", c.tenant, err)
		}
		fmt.Printf("Silence added for '%s' tenant: %s", c.tenant, postOk.Payload.SilenceID)
		return nil
	} else if c.tenantFile != "" {

		tenants, err := readTenantFromFile(c.tenantFile)
		if err != nil {
			return err
		}

		for _, t := range tenants {
			httpConfig = setHTTPTenantHeader(httpConfig, t, c.tenantHTTPHeader)
			amclient := NewAlertmanagerClient(alertmanagerURL, *httpConfig)

			postOk, err := amclient.Silence.PostSilences(silenceParams)
			if err != nil {
				fmt.Printf("Unable to add silence for '%s' tenant: %v\n", t, err)
				continue
			}
			fmt.Printf("Silence added for '%s' tenant: %s\n", t, postOk.Payload.SilenceID)

		}
	} else {
		amclient := NewAlertmanagerClient(alertmanagerURL, *httpConfig)

		postOk, err := amclient.Silence.PostSilences(silenceParams)
		if err != nil {
			return fmt.Errorf("Unable to add silence: %v", err)
		}
		fmt.Printf("Silence added: %s", postOk.Payload.SilenceID)
		return err
	}
	return nil
}

func readTenantFromFile(tenantFile string) ([]string, error) {
	var tenants []string

	readFile, err := os.Open(tenantFile)
	if err != nil {
		return tenants, fmt.Errorf("Unable to read tenant file '%s': %v", tenantFile, err)
	}

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)
	for fileScanner.Scan() {
		tenants = append(tenants, fileScanner.Text())
	}
	readFile.Close()

	return tenants, nil
}

func setHTTPTenantHeader(httpConfig *promconfig.HTTPClientConfig, tenant, tenantHTTPHeader string) *promconfig.HTTPClientConfig {
	if httpConfig.HTTPHeaders == nil {
		httpConfig.HTTPHeaders = &promconfig.Headers{
			Headers: map[string]promconfig.Header{
				tenantHTTPHeader: {Values: []string{tenant}},
			},
		}
	} else {
		httpConfig.HTTPHeaders.Headers[tenantHTTPHeader] = promconfig.Header{Values: []string{tenant}}
	}
	return httpConfig
}
