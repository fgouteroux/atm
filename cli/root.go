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
	"net/url"
	"os"
	"path"
	"time"

	"github.com/alecthomas/kingpin/v2"
	clientruntime "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"

	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/cli/config"
	"github.com/prometheus/alertmanager/cli/format"
)

var (
	alertmanagerURL *url.URL
	timeout         time.Duration
	httpConfigFile  string

	configFiles = []string{os.ExpandEnv("$HOME/.config/atm/config.yml"), "/etc/atm/config.yml"}
	legacyFlags = map[string]string{"comment_required": "require-comment"}
)

func requireAlertManagerURL(pc *kingpin.ParseContext) error {
	// Return without error if any help flag is set.
	for _, elem := range pc.Elements {
		f, ok := elem.Clause.(*kingpin.FlagClause)
		if !ok {
			continue
		}
		name := f.Model().Name
		if name == "help" || name == "help-long" || name == "help-man" {
			return nil
		}
	}
	if alertmanagerURL == nil {
		kingpin.Fatalf("required flag --alertmanager.url not provided")
	}
	return nil
}

const (
	defaultAmHost      = "localhost"
	defaultAmPort      = "9093"
	defaultAmApiv2path = "/api/v2"
)

// NewAlertmanagerClientConfig initializes an alertmanager client config with the given URL.
func NewAlertmanagerClientConfig() *promconfig.HTTPClientConfig {
	var httpConfig *promconfig.HTTPClientConfig
	if httpConfigFile != "" {
		var err error
		httpConfig, _, err = promconfig.LoadHTTPConfigFile(httpConfigFile)
		if err != nil {
			kingpin.Fatalf("failed to load HTTP config file: %v", err)
		}
	} else {
		httpConfig = &promconfig.HTTPClientConfig{}
	}
	return httpConfig
}

// NewAlertmanagerClient initializes an alertmanager client with the given URL.
func NewAlertmanagerClient(amURL *url.URL, httpConfig promconfig.HTTPClientConfig) *client.AlertmanagerAPI {
	address := defaultAmHost + ":" + defaultAmPort
	schemes := []string{"http"}

	if amURL.Host != "" {
		address = amURL.Host // URL documents host as host or host:port
	}
	if amURL.Scheme != "" {
		schemes = []string{amURL.Scheme}
	}

	cr := clientruntime.New(address, path.Join(amURL.Path, defaultAmApiv2path), schemes)

	if amURL.User != nil && httpConfigFile != "" {
		kingpin.Fatalf("basic authentication and http.config.file are mutually exclusive")
	}

	if amURL.User != nil {
		password, _ := amURL.User.Password()
		cr.DefaultAuthentication = clientruntime.BasicAuth(amURL.User.Username(), password)
	}

	httpclient, err := promconfig.NewClientFromConfig(httpConfig, "atm")
	if err != nil {
		kingpin.Fatalf("failed to create a new HTTP client: %v", err)
	}
	cr = clientruntime.NewWithClient(address, path.Join(amURL.Path, defaultAmApiv2path), schemes, httpclient)

	return client.New(cr, strfmt.Default)
}

// Execute is the main function for the atm command.
func Execute() {
	app := kingpin.New("atm", helpRoot).UsageWriter(os.Stdout)

	format.InitFormatFlags(app)

	app.Flag("alertmanager.url", "Alertmanager to talk to").URLVar(&alertmanagerURL)
	app.Flag("timeout", "Timeout for the executed command").Default("30s").DurationVar(&timeout)
	app.Flag("http.config.file", "HTTP client configuration file for atm to connect to Alertmanager.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFile)

	app.Version(version.Print("atm"))
	app.GetFlag("help").Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)

	resolver, err := config.NewResolver(configFiles, legacyFlags)
	if err != nil {
		kingpin.Fatalf("could not load config file: %v\n", err)
	}

	configureSilenceCmd(app)

	err = resolver.Bind(app, os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}

	_, err = app.Parse(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}
}

const (
	helpRoot = `Alertmanager silences distributor.

Config File:
The atm tool will read a config file in YAML format from one of two
default config locations: $HOME/.config/atm/config.yml or
/etc/atm/config.yml

All flags can be given in the config file, but the following are the suited for
static configuration:

	alertmanager.url
		Set a default alertmanager url for each request

	author
		Set a default author value for new silences. If this argument is not
		specified then the username will be used

	require-comment
		Bool, whether to require a comment on silence creation. Defaults to true

	http.config.file
		HTTP client configuration file for atm to connect to Alertmanager.
		The format is https://prometheus.io/docs/alerting/latest/configuration/#http_config.
`
)
