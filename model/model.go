package model

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewClient() (client *dynamodb.Client, err error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == dynamodb.ServiceID && region == "local" {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           "http://localhost:8000",
				SigningRegion: "local",
			}, nil
		}

		// returning EndpointNotFoundError will allow the service to fallback to it's default resolution
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithEndpointResolverWithOptions(customResolver),
		// config.WithLogLevel(aws.LogDebugWithHTTPBody),
	)
	if err != nil {
		return
	}

	// Create the DynamoDB client
	client = dynamodb.NewFromConfig(cfg)

	return
}

func MakeTables(client *dynamodb.Client) (err error) {
	ctx := context.TODO()

	// List all the tables in the account
	output, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		panic(err)
	}

	// Print the names of all the tables
	tables := make(map[string]string, len(output.TableNames))
	for _, tableName := range output.TableNames {
		tables[tableName] = tableName
	}

	if _, exists := tables[SummaryModelTableName]; !exists {
		err = makeSummariesTable(client)
		if err != nil {
			return
		}
	}

	if _, exists := tables[FeedbackModelTableName]; !exists {
		err = makeFeedbackTable()
		if err != nil {
			return
		}
	}

	return
}

func makeFeedbackTable() (err error) {
	return
}

var (
	logger *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "[M] ", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
}
