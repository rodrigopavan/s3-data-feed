package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
)

func startQueuePooling(broker *Broker) {
	awsKey := getEnvDefault("AWS_KEY", "test")
	awsSecret := getEnvDefault("AWS_SECRET", "test")
	awsURL := getEnvDefault("AWS_URL", "http://localhost:4566")
	awsRegion := getEnvDefault("AWS_REGION", "us-east-1")
	awsSNSTopic := getEnvDefault("AWS_TOPIC_ARN", "arn:aws:sns:us-east-1:000000000000:filecreate")
	awsSQSQueue := getEnvDefault("AWS_SQS_QUEUE", "march")

	waitTime := 20
	cfg := aws.NewConfig().WithCredentials(
		credentials.NewStaticCredentials(awsKey, awsSecret, ""),
	).WithEndpoint(awsURL).WithRegion(awsRegion)
	s, err := session.NewSession(cfg)
	if err != nil {
		log.Fatal(err)
	}

	queue := sqs.New(s)
	snsClient := sns.New(s)
	s3Client := s3.New(s)
	downloader := s3manager.NewDownloaderWithClient(s3Client)

	var qUrl *string

	result, err := queue.GetQueueUrl(&sqs.GetQueueUrlInput{
		QueueName: aws.String(awsSQSQueue),
	})
	if err != nil {
		fmt.Println("Queue not found, let's create")
		result, err := queue.CreateQueue(&sqs.CreateQueueInput{
			QueueName: aws.String(awsSQSQueue),
			Attributes: aws.StringMap(map[string]string{
				"ReceiveMessageWaitTimeSeconds": strconv.Itoa(aws.IntValue(&waitTime)),
			}),
		})
		if err != nil {
			log.Fatal(err)
		}
		qUrl = result.QueueUrl
		fmt.Println("Subscribing queue in the topic")
		_, err = snsClient.Subscribe(&sns.SubscribeInput{
			TopicArn: aws.String(awsSNSTopic),
			Protocol: aws.String("sqs"),
			Endpoint: qUrl,
		})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		qUrl = result.QueueUrl
	}

	// enabling long-pooling in the queue (MUST BE 20 seconds)
	/*_, err = queue.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueUrl: qUrl,
		Attributes: aws.StringMap(map[string]string{
			"ReceiveMessageWaitTimeSeconds": strconv.Itoa(aws.IntValue(&waitTime)),
		}),
	})
	if err != nil {
		log.Fatal(err)
	}*/

	go func() {
		for {
			poolMessage(queue, qUrl, waitTime, downloader, broker)
		}
	}()
}

func poolMessage(queue *sqs.SQS, qUrl *string, waitTime int, downloader *s3manager.Downloader, broker *Broker) {
	fmt.Println("Pooling..")
	msgResult, err := queue.ReceiveMessage(&sqs.ReceiveMessageInput{
		AttributeNames: []*string{
			aws.String(sqs.MessageSystemAttributeNameSentTimestamp),
		},
		MessageAttributeNames: []*string{
			aws.String(sqs.QueueAttributeNameAll),
		},
		QueueUrl:            qUrl,
		MaxNumberOfMessages: aws.Int64(10),
		VisibilityTimeout:   aws.Int64(5),
		WaitTimeSeconds:     aws.Int64(int64(waitTime)),
	})
	if err != nil {
		fmt.Printf("Failure to retrieve events from SQS due to %v\n", err)
		return
	}

	for _, msg := range msgResult.Messages {
		var snsEvent SNSEvent
		var sqsEvent SQSEvent
		err := json.Unmarshal([]byte(*msg.Body), &snsEvent)
		if err != nil {
			fmt.Printf("Failure to unmarshal SNS event due to %v\n", err)
			continue
		}

		err = json.Unmarshal([]byte(snsEvent.Message), &sqsEvent)
		if err != nil {
			fmt.Printf("Failure to unmarshal S3 event due to %v\n", err)
			continue
		}
		for _, r := range sqsEvent.Records {
			var fileContent aws.WriteAtBuffer
			_, err := downloader.Download(&fileContent, &s3.GetObjectInput{
				Bucket: aws.String(r.S3.Bucket.Name),
				Key:    aws.String(r.S3.Object.Key),
			})
			if err != nil {
				fmt.Printf("Failure to download file [%s] from bucket [%s] due to %v\n", r.S3.Object.Key, r.S3.Bucket.Name, err)
				continue
			}
			fmt.Printf("Fanout event [%s] to consumers\n", r.EventName)
			broker.Notifier <- fileContent.Bytes()
		}

		_, err = queue.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      qUrl,
			ReceiptHandle: msg.ReceiptHandle,
		})
		if err != nil {
			fmt.Printf("Failure to delete queue messagedue to %v\n", err)
		}
	}
}

func getEnvDefault(key string, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
