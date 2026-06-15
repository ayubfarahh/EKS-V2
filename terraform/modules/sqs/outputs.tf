output "queue_url" {
  value = aws_sqs_queue.order-events.url
}

output "queue_arn" {
  value = aws_sqs_queue.order-events.arn
}

