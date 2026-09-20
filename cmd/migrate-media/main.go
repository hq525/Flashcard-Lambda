// migrate-media validates legacy image records by default. Add --apply and an
// explicit source-bucket allowlist to copy normalized media and attach its key.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"flashcard_lambda/internal/migration"
	"flashcard_lambda/internal/storage"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type options struct {
	config     migration.Config
	region     string
	reportPath string
}

func parseOptions(args []string, usage io.Writer) (options, error) {
	var options options
	var sources string
	flags := flag.NewFlagSet("migrate-media", flag.ContinueOnError)
	flags.SetOutput(usage)
	flags.StringVar(&options.config.Table, "table", "", "DynamoDB table name (required)")
	flags.StringVar(&options.config.Bucket, "bucket", "", "destination media bucket (required)")
	flags.StringVar(&sources, "source-buckets", "", "comma-separated approved source buckets (required with --apply; dry run defaults to destination)")
	flags.BoolVar(&options.config.Apply, "apply", false, "copy and attach images; default only validates metadata")
	flags.IntVar(&options.config.MaxPages, "max-pages", migration.DefaultMaxPages, "maximum 100-item scan pages (1-10000)")
	flags.StringVar(&options.region, "region", "", "AWS region (defaults to configured AWS region)")
	flags.StringVar(&options.reportPath, "report", "", "optional new JSON report file; refuses to overwrite an existing file")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, errors.New("unexpected positional arguments")
	}
	if options.config.MaxPages == 0 {
		return options, errors.New("--max-pages must be between 1 and 10000")
	}
	if sources != "" {
		for _, bucket := range strings.Split(sources, ",") {
			options.config.SourceBuckets = append(options.config.SourceBuckets, strings.TrimSpace(bucket))
		}
	}
	return options, options.config.Validate()
}

func writeReport(path string, report migration.Report) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("could not create report file; it must be a new writable path")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(report)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("could not finish writing report file")
	}
	return nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	options, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if options.reportPath != "" {
		if _, err := os.Lstat(options.reportPath); !errors.Is(err, os.ErrNotExist) {
			return errors.New("report path already exists or cannot be inspected")
		}
	}
	var loadOptions []func(*awsconfig.LoadOptions) error
	if options.region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(options.region))
	}
	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return errors.New("could not load AWS configuration")
	}
	if sdkConfig.Region == "" {
		return errors.New("an AWS region must be configured or supplied with --region")
	}
	sdkConfig.HTTPClient = &http.Client{Timeout: 45 * time.Second}
	sdkConfig.RetryMaxAttempts = 3
	db := dynamodb.NewFromConfig(sdkConfig)
	objects := s3.NewFromConfig(sdkConfig)
	writer := storage.NewS3ImageStore(objects, options.config.Bucket)
	report, runErr := migration.Run(ctx, options.config, db, objects, writer)
	if options.reportPath != "" {
		if err := writeReport(options.reportPath, report); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return errors.New("could not output migration report")
	}
	if runErr != nil {
		return runErr
	}
	if report.Errors > 0 {
		return errors.New("migration has unresolved records; review the sanitized error codes")
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
