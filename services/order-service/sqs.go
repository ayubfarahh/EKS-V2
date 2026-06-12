// sqs.go - SQS helper
//
// Wraps the AWS SQS SDK so the rest of the code doesn't deal with it directly.
// Handles local vs production automatically: if AWS_ENDPOINT_URL is set
// (Docker Compose -> LocalStack), it talks to LocalStack. If not (EKS via
// IRSA), it talks to real SQS. No code changes between environments.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SQSClient struct {
	sqs      *sqs.Client
	queueURL string
}

type SQSMessage struct {
	Type      string                 `json:"type"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp string                 `json:"timestamp"`
}

func newSQSClient() (*SQSClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(getEnv("AWS_REGION", getEnv("AWS_DEFAULT_REGION", "eu-west-1"))),
	)
	if err != nil {
		return nil, err
	}
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")
	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpointURL != "" {
			o.BaseEndpoint = aws.String(endpointURL)
		}
	})
	return &SQSClient{
		sqs:      client,
		queueURL: os.Getenv("SQS_QUEUE_URL"),
	}, nil
}

func (c *SQSClient) publish(ctx context.Context, eventType string, payload map[string]interface{}) error {
	msg := SQSMessage{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(c.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		log.Printf("SQS publish error [%s]: %v", eventType, err)
		return err
	}
	log.Printf("SQS published: %s", eventType)
	return nil
}

func (c *SQSClient) receive(ctx context.Context) ([]SQSMessage, []string, error) {
	result, err := c.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: 10,
		WaitTimeSeconds:     20,
	})
	if err != nil {
		return nil, nil, err
	}
	var messages []SQSMessage
	var receiptHandles []string
	for _, m := range result.Messages {
		var msg SQSMessage
		if err := json.Unmarshal([]byte(*m.Body), &msg); err != nil {
			log.Printf("Failed to parse SQS message: %v", err)
			continue
		}
		messages = append(messages, msg)
		receiptHandles = append(receiptHandles, *m.ReceiptHandle)
	}
	return messages, receiptHandles, nil
}

func (c *SQSClient) delete(ctx context.Context, receiptHandle string) error {
	_, err := c.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	return err
}

func (c *SQSClient) isConfigured() bool {
	return c.queueURL != ""
}

// Payload helpers - JSON numbers always unmarshal as float64
func sqsGetString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func sqsGetFloat(payload map[string]interface{}, key string) float64 {
	if v, ok := payload[key].(float64); ok {
		return v
	}
	return 0
}

func sqsGetInt(payload map[string]interface{}, key string) int {
	if v, ok := payload[key].(float64); ok {
		return int(v)
	}
	return 0
}
