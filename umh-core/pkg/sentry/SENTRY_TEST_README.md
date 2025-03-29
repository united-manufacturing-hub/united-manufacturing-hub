# Sentry Test

This directory contains a test to verify Sentry integration and troubleshoot why reports may not be showing up in Sentry.

## Running the Test

To run the test:

```bash
cd umh-core
go test -v -ginkgo.focus "Manually sends a test message to Sentry" ./pkg/sentry
```

## What the Test Does

The test:

1. Initializes Sentry with a test version ("0.0.0-test")
2. Sends several test messages to Sentry with a timestamp for identification:
   - A warning message via `ReportIssue`
   - An error message via `ReportIssue`
   - A formatted warning message via `ReportIssuef`
3. Waits 5 seconds for messages to be sent to Sentry
4. Outputs information about what messages to look for in the Sentry dashboard

## Troubleshooting

If messages don't appear in Sentry, check:

1. Network connectivity to Sentry servers
2. DSN configuration in `sentry.go`
3. Authentication issues
4. Rate limiting or filtering in Sentry
5. Firewall or proxy settings

## Modifying the Test

You can modify the test in `sentry_test.go` to try different types of messages or error scenarios. 