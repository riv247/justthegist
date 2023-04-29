package model

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const SummaryModelTableName = "Summaries"
const FeedbackModelTableName = "Feedback"

func makeSummariesTable(client *dynamodb.Client) (err error) {
	logger.Println("makeSummariesTable")

	tableName := "Summaries"
	ctx := context.TODO()

	_, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &tableName,
	})
	if err != nil {
		// table does not exist
		if errors.As(err, new(*types.ResourceNotFoundException)) {
			input := &dynamodb.CreateTableInput{
				AttributeDefinitions: []types.AttributeDefinition{
					{
						AttributeName: aws.String("provider_id"),
						AttributeType: types.ScalarAttributeTypeS,
					},
					{
						AttributeName: aws.String("created_at"),
						AttributeType: types.ScalarAttributeTypeS,
					},
				},
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("provider_id"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("created_at"),
						KeyType:       types.KeyTypeRange,
					},
				},
				BillingMode: types.BillingModePayPerRequest, // Use pay-per-request billing
				TableName:   aws.String(tableName),
			}

			// Try to create the table
			_, err = client.CreateTable(ctx, input)
			if err != nil {
				logger.Println("failed creating table:", tableName)
				logger.Println("ERROR:", err.Error())

				if errors.Is(err, new(types.ResourceInUseException)) {
					logger.Println("table already exists:", tableName)
					return
				}

				if errors.As(err, new(*types.ResourceInUseException)) {
					logger.Println("table already exists:", tableName)
					return
				}
			}

			logger.Println("table created:", tableName)
			return
		}
	}

	logger.Println("table already exists:", tableName)
	return
}

type SummaryModel struct {
	ProviderID    string    `dynamodbav:"provider_id" json:"provider_id,omitempty"`
	Provider      string    `dynamodbav:"provider" json:"provider,omitempty"`
	PromptVersion string    `dynamodbav:"prompt_version" json:"prompt_version,omitempty"`
	Context       string    `dynamodbav:"context" json:"context,omitempty"`
	Summary       string    `dynamodbav:"summary" json:"summary,omitempty"`
	TLDR          string    `dynamodbav:"tldr" json:"tldr,omitempty"`
	CreatedAt     time.Time `dynamodbav:"created_at" json:"created_at,omitempty"`
	UpdatedAt     time.Time `dynamodbav:"updated_at" json:"updated_at,omitempty"`
}

func (s *SummaryModel) Get(client *dynamodb.Client, provider, providerID string) (err error) {
	ctx := context.TODO()

	// Create the input for the query
	input := &dynamodb.GetItemInput{
		TableName: aws.String(SummaryModelTableName),
		Key: map[string]types.AttributeValue{
			"provider_id": &types.AttributeValueMemberS{
				Value: providerID,
			},
			"provider": &types.AttributeValueMemberS{
				Value: provider,
			},
		},
	}

	// Retrieve the item from DynamoDB. If no matching item is found
	// return nil.
	result, err := client.GetItem(ctx, input)
	if err != nil {
		return
	}

	if result.Item == nil {
		return
	}

	// Unmarshal the result into a SummaryModel instance
	err = attributevalue.UnmarshalMap(result.Item, s)
	if err != nil {
		return
	}

	return
}

func (s *SummaryModel) Save(client *dynamodb.Client) (err error) {
	ctx := context.TODO()

	// Marshall the item into a map
	itemMap, err := attributevalue.MarshalMap(s)
	if err != nil {
		return
	}

	// Create a new input struct to put an item in the table
	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(SummaryModelTableName),
		Item:      itemMap,
		// ConditionExpression: aws.String("attribute_exists(provider_id) AND attribute_exists(provider)"),
	}

	// Put the item in the table
	_, err = client.PutItem(ctx, putInput)
	if err != nil {
		return
	}

	return
}
