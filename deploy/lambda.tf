terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.74"
    }
  }
}

provider "aws" {
  region              = var.aws_region
  allowed_account_ids = ["780219548054"]
}

variable "schedule_enabled" {
  type        = bool
  default     = false
  description = "Enable the five-minute schedule after integration credentials are configured."
}

variable "integration_environment" {
  type        = map(string)
  default     = {}
  sensitive   = true
  description = "Integration environment variables. Values are stored in Terraform state; supply through an untracked tfvars file."
}

resource "aws_iam_role" "grass_lambda" {
  name = "grass-lambda"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_cloudwatch_log_group" "grass_lambda" {
  name              = "/aws/lambda/grass"
  retention_in_days = 30
}

resource "aws_iam_role_policy" "grass_lambda" {
  role = aws_iam_role.grass_lambda.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem", "dynamodb:UpdateItem"]
        Resource = aws_dynamodb_table.social_search.arn
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.grass_lambda.arn}:*"
      }
    ]
  })
}

resource "aws_lambda_function" "grass" {
  function_name    = "grass"
  role             = aws_iam_role.grass_lambda.arn
  filename         = "${path.module}/../dist/grass-lambda.zip"
  source_code_hash = filebase64sha256("${path.module}/../dist/grass-lambda.zip")
  runtime          = "provided.al2023"
  handler          = "bootstrap"
  architectures    = ["arm64"]
  memory_size      = 256
  timeout          = 300

  environment {
    variables = merge(var.integration_environment, {
      GRASS_KEYWORDS           = "tailscale,leebriggs,leebriggs.co.uk"
      GRASS_SEARCHERS          = "reddit,hackernews,bluesky,youtube,fediverse"
      GRASS_BOTS               = "print,discord"
      SOCIAL_SEARCH_TABLE_NAME = aws_dynamodb_table.social_search.name
      FEDIVERSE_INSTANCES      = "https://hachyderm.io"
    })
  }

  depends_on = [aws_iam_role_policy.grass_lambda]
}

resource "aws_cloudwatch_event_rule" "grass" {
  name                = "grass-every-five-minutes"
  schedule_expression = "rate(5 minutes)"
  state               = var.schedule_enabled ? "ENABLED" : "DISABLED"
}

resource "aws_lambda_permission" "schedule" {
  statement_id  = "AllowEventBridgeSchedule"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.grass.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.grass.arn
}

resource "aws_cloudwatch_event_target" "grass" {
  rule       = aws_cloudwatch_event_rule.grass.name
  arn        = aws_lambda_function.grass.arn
  depends_on = [aws_lambda_permission.schedule]
}

output "lambda_function_arn" {
  value = aws_lambda_function.grass.arn
}

output "schedule_enabled" {
  value = var.schedule_enabled
}
