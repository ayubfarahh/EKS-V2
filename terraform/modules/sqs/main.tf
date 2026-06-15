resource "aws_sqs_queue" "order-events" {
  name                      = "order-events-queue"
  delay_seconds             = 0
  receive_wait_time_seconds = 20
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.order-events-dlq.arn
    maxReceiveCount     = 4
  })

 
}

resource "aws_sqs_queue" "order-events-dlq" {
  name = "order-events-deadletter-queue"
}

resource "aws_sqs_queue_redrive_allow_policy" "order-events-redrive-allow-policy" {
  queue_url = aws_sqs_queue.order-events-dlq.id

  redrive_allow_policy = jsonencode({
    redrivePermission = "byQueue",
    sourceQueueArns   = [aws_sqs_queue.order-events.arn]
  })
}