resource "aws_dynamodb_table" "social_search" {
  name           = var.search_table_name
  billing_mode   = "PROVISIONED"
  read_capacity  = var.read_capacity
  write_capacity = var.write_capacity
  hash_key       = "Platform"     # Partition Key
  range_key      = "SortKey"      # Sort Key

  attribute {
    name = "Platform"
    type = "S" # String
  }

  attribute {
    name = "SortKey"
    type = "S" # String
  }

  attribute {
    name = "Timestamp"
    type = "N" # Number (for Unix timestamp)
  }

  attribute {
    name = "Keyword"
    type = "S" # String
  }

  global_secondary_index {
    name               = "KeywordIndex"
    hash_key           = "Keyword"
    projection_type    = "ALL"
    read_capacity      = var.read_capacity
    write_capacity     = var.write_capacity
  }

  global_secondary_index {
    name               = "TimestampIndex"
    hash_key           = "Timestamp"
    projection_type    = "ALL"
    read_capacity      = var.read_capacity
    write_capacity     = var.write_capacity
  }

  tags = {
    Name        = var.search_table_name
    Environment = "production"
  }
}
