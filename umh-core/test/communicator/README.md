# Communicator Tests

This directory contains test files for the UMH Communicator component.

## Overview

The communicator is responsible for:
1. Establishing a connection with the backend
2. Subscribing users to receive updates
3. Sending status messages to subscribed users
4. Receiving and processing commands

## Available Tests

### Subscribe and Receive Test

The `subscribe_and_receive_test.go` file tests the subscription mechanism. It:

1. Sets up a mock HTTP endpoint using Gock
2. Creates a CommunicationState with all required components
3. Sends a Subscribe message to the endpoint
4. Verifies that:
   - The subscriber is added to the subscriber list
   - Status messages are sent to the subscriber by capturing and inspecting outgoing messages
   - The content of status messages follows the expected format

## Running the Tests

You can run all the tests in this directory using:

```bash
cd umh-core
go test ./test/communicator/... -v
```

Or run a specific test using:

```bash
cd umh-core
go test ./test/communicator -run "Subscribe" -v
```

## Test Structure

Each test follows this structure:
1. `BeforeEach`: Sets up the mock HTTP endpoints and initializes components
2. `AfterEach`: Cleans up resources
3. Test cases: One or more `It` blocks that define test cases
4. Assertions: Using Gomega assertions to verify expected behavior 