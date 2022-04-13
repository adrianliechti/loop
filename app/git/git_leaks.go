package git

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var leaksCommand = &cli.Command{
	Name:  "leaks",
	Usage: "find data leaks in repository",

	Action: func(c *cli.Context) error {
		return leaks(c.Context)
	},
}

func leaks(ctx context.Context) error {
	path, err := os.Getwd()

	if err != nil {
		return err
	}

	// config, err := ioutil.TempFile("", "gitleaks-")

	// if err != nil {
	// 	return err
	// }

	// defer os.Remove(config.Name())

	// if _, err = config.Write([]byte(gitleaksConfig)); err != nil {
	// 	return err
	// }

	// if err := config.Close(); err != nil {
	// 	return err
	// }

	options := docker.RunOptions{
		Volumes: map[string]string{
			path: "/src",
			//config.Name(): "/config",
		},
	}

	args := []string{
		"detect",
		"-v",
		"--source=/src",
		//"--config=/config",
	}

	return docker.RunInteractive(ctx, "zricethezav/gitleaks:v8.6.1", options, args...)
}

// const gitleaksConfig = `
// title = "gitleaks config"

// [[rules]]
// 	description = "AWS Access Key"
// 	regex = '''(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}'''
// 	tags = ["key", "AWS"]

// [[rules]]
// 	description = "AWS Secret Key"
// 	regex = '''(?i)aws(.{0,20})?(?-i)['\"][0-9a-zA-Z\/+]{40}['\"]'''
// 	tags = ["key", "AWS"]

// [[rules]]
// 	description = "AWS MWS key"
// 	regex = '''amzn\.mws\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'''
// 	tags = ["key", "AWS", "MWS"]

// [[rules]]
// 	description = "Facebook Secret Key"
// 	regex = '''(?i)(facebook|fb)(.{0,20})?(?-i)['\"][0-9a-f]{32}['\"]'''
// 	tags = ["key", "Facebook"]

// [[rules]]
// 	description = "Facebook Client ID"
// 	regex = '''(?i)(facebook|fb)(.{0,20})?['\"][0-9]{13,17}['\"]'''
// 	tags = ["key", "Facebook"]

// [[rules]]
// 	description = "Twitter Secret Key"
// 	regex = '''(?i)twitter(.{0,20})?[0-9a-z]{35,44}'''
// 	tags = ["key", "Twitter"]

// [[rules]]
// 	description = "Twitter Client ID"
// 	regex = '''(?i)twitter(.{0,20})?[0-9a-z]{18,25}'''
// 	tags = ["client", "Twitter"]

// [[rules]]
// 	description = "Github"
// 	regex = '''(?i)github(.{0,20})?(?-i)[0-9a-zA-Z]{35,40}'''
// 	tags = ["key", "Github"]

// [[rules]]
// 	description = "LinkedIn Client ID"
// 	regex = '''(?i)linkedin(.{0,20})?(?-i)[0-9a-z]{12}'''
// 	tags = ["client", "LinkedIn"]

// [[rules]]
// 	description = "LinkedIn Secret Key"
// 	regex = '''(?i)linkedin(.{0,20})?[0-9a-z]{16}'''
// 	tags = ["secret", "LinkedIn"]

// [[rules]]
// 	description = "Slack"
// 	regex = '''xox[baprs]-([0-9a-zA-Z]{10,48})?'''
// 	tags = ["key", "Slack"]

// [[rules]]
// 	description = "Asymmetric Private Key"
// 	regex = '''-----BEGIN ((EC|PGP|DSA|RSA|OPENSSH) )?PRIVATE KEY( BLOCK)?-----'''
// 	tags = ["key", "AsymmetricPrivateKey"]

// [[rules]]
// 	description = "Google API key"
// 	regex = '''AIza[0-9A-Za-z\\-_]{35}'''
// 	tags = ["key", "Google"]

// [[rules]]
// 	description = "Google (GCP) Service Account"
// 	regex = '''"type": "service_account"'''
// 	tags = ["key", "Google"]

// [[rules]]
// 	description = "Heroku API key"
// 	regex = '''(?i)heroku(.{0,20})?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'''
// 	tags = ["key", "Heroku"]

// [[rules]]
// 	description = "MailChimp API key"
// 	regex = '''(?i)(mailchimp|mc)(.{0,20})?[0-9a-f]{32}-us[0-9]{1,2}'''
// 	tags = ["key", "Mailchimp"]

// [[rules]]
// 	description = "Mailgun API key"
// 	regex = '''((?i)(mailgun|mg)(.{0,20})?)?key-[0-9a-z]{32}'''
// 	tags = ["key", "Mailgun"]

// [[rules]]
// 	description = "PayPal Braintree access token"
// 	regex = '''access_token\$production\$[0-9a-z]{16}\$[0-9a-f]{32}'''
// 	tags = ["key", "Paypal"]

// [[rules]]
// 	description = "Picatic API key"
// 	regex = '''sk_live_[0-9a-z]{32}'''
// 	tags = ["key", "Picatic"]

// [[rules]]
// 	description = "SendGrid API Key"
// 	regex = '''SG\.[\w_]{16,32}\.[\w_]{16,64}'''
// 	tags = ["key", "SendGrid"]

// [[rules]]
// 	description = "Slack Webhook"
// 	regex = '''https://hooks.slack.com/services/T[a-zA-Z0-9_]{8}/B[a-zA-Z0-9_]{8}/[a-zA-Z0-9_]{24}'''
// 	tags = ["key", "slack"]

// [[rules]]
// 	description = "Stripe API key"
// 	regex = '''(?i)stripe(.{0,20})?[sr]k_live_[0-9a-zA-Z]{24}'''
// 	tags = ["key", "Stripe"]

// [[rules]]
// 	description = "Square access token"
// 	regex = '''sq0atp-[0-9A-Za-z\-_]{22}'''
// 	tags = ["key", "square"]

// [[rules]]
// 	description = "Square OAuth secret"
// 	regex = '''sq0csp-[0-9A-Za-z\\-_]{43}'''
// 	tags = ["key", "square"]

// [[rules]]
// 	description = "Twilio API key"
// 	regex = '''(?i)twilio(.{0,20})?SK[0-9a-f]{32}'''
// 	tags = ["key", "twilio"]

// [[rules]]
// 	description = "Dynatrace ttoken"
// 	regex = '''dt0[a-zA-Z]{1}[0-9]{2}\.[A-Z0-9]{24}\.[A-Z0-9]{64}'''
// 	tags = ["key", "Dynatrace"]

// [[rules]]
// 	description = "Shopify shared secret"
// 	regex = '''shpss_[a-fA-F0-9]{32}'''
// 	tags = ["key", "Shopify"]

// [[rules]]
// 	description = "Shopify access token"
// 	regex = '''shpat_[a-fA-F0-9]{32}'''
// 	tags = ["key", "Shopify"]

// [[rules]]
// 	description = "Shopify custom app access token"
// 	regex = '''shpca_[a-fA-F0-9]{32}'''
// 	tags = ["key", "Shopify"]

// [[rules]]
// 	description = "Shopify private app access token"
// 	regex = '''shppa_[a-fA-F0-9]{32}'''
// 	tags = ["key", "Shopify"]

// [allowlist]
// 	description = "Allowlisted files"
// 	files = ['''^\.?gitleaks.toml$''',
// 	'''(.*?)(png|jpg|gif|doc|docx|pdf|bin|xls|pyc|zip)$''',
// 	'''(go.mod|go.sum)$''']

// [[rules]]
// 	description = "Email"
// 	regex = '''[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,4}'''
// 	tags = ["email"]

// [[rules]]
// 	description = "High Entropy"
// 	regex = '''[0-9a-zA-Z-_!{}/=]{4,120}'''
//   	file = '''(?i)(dump.sql|high-entropy-misc.txt)$'''
// 	tags = ["entropy"]
//     [[rules.Entropies]]
//         Min = "4.3"
//         Max = "7.0"
//     [rules.allowlist]
//         description = "ignore ssh key and pems"
//         files = ['''(pem|ppk|env)$''']
// 		paths = ['''(.*)?ssh''']

// [[rules]]
// 	description = "Files with keys and credentials"
// 	file = '''(?i)(id_rsa|passwd|id_rsa.pub|pgpass|pem|key|shadow)'''
// `
