# s3-data-feed

This project is used to fanout S3 file content to connected clients over SSE when the file is inserted on S3 (s3:ObjectCreated:Put event).

## data pipeline

When a file is sent to S3, a notification event is generated to SNS, then the event is sent to SQS subscribed to SNS Topic. The s3-data-feed application consumes the SQS, when received new events, the application get the file content on S3 (based on the event metadata) and send the file content to all clients connected over SSE to the application.  

## prepare environment

You can use [Localstack](https://github.com/localstack/localstack) to test this application.

Using [AWS Cli](https://github.com/aws/aws-cli), execute:

```
#create S3 bucket
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket

#create SNS topic
aws --endpoint-url=http://localhost:4566 sns create-topic --name filecreate

#configure Event Notification of S3 (you can use the notification.json on this repo as a example)
aws --endpoint-url=http://localhost:4566 s3api put-bucket-notification-configuration --bucket march --notification-configuration file://notification.json

#to send files to S3, you can use
aws --endpoint-url=http://localhost:4572 s3 cp test.csv s3://my-bucket

```

There's a few ENV vars that can be used in this application:
```
"AWS_KEY" default "test"
"AWS_SECRET" default "test"
"AWS_URL" default "http://localhost:4566"
"AWS_REGION" default "us-east-1"
"AWS_TOPIC_ARN" default "arn:aws:sns:us-east-1:000000000000:filecreate"
"AWS_SQS_QUEUE" default "march" -- this is only the name of SQS that the application will create
```

During the application startup it will automatically create a SQS and subscribe it to SNS topic.

## how to use

With application running (`go run .`), you can use curl to consume events:
```
curl localhost:3000
```

After that, send a file to S3:
```
aws --endpoint-url=http://localhost:4572 s3 cp test.csv s3://my-bucket
```

You should see the file content be delivered in the curl execution.
