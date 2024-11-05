# deploy/variables.tf
variable "aws_region" {
  description = "The AWS region to deploy resources in"
  type        = string
  default     = "us-west-2"
}

variable "search_table_name" {
  description = "Name of the DynamoDB table for search results and last search time tracking"
  type        = string
  default     = "grass"
}

variable "read_capacity" {
  description = "Read capacity for the DynamoDB table"
  type        = number
  default     = 5
}

variable "write_capacity" {
  description = "Write capacity for the DynamoDB table"
  type        = number
  default     = 5
}

