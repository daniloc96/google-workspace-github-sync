#!/bin/bash
set -euo pipefail

ENDPOINT="http://localhost:8000"
TABLE_NAME="invitation-mappings"
REGION="eu-west-1"

export AWS_ACCESS_KEY_ID=local
export AWS_SECRET_ACCESS_KEY=local
export AWS_DEFAULT_REGION=$REGION

echo "🚀 Starting DynamoDB Local..."
docker compose up -d

echo "⏳ Waiting for DynamoDB Local to be ready..."
until aws dynamodb list-tables --endpoint-url $ENDPOINT --region $REGION > /dev/null 2>&1; do
  sleep 1
done

echo "📦 Creating table: $TABLE_NAME"
aws dynamodb create-table \
  --endpoint-url $ENDPOINT \
  --table-name $TABLE_NAME \
  --attribute-definitions \
    AttributeName=pk,AttributeType=S \
    AttributeName=sk,AttributeType=S \
    AttributeName=gsi1pk,AttributeType=S \
    AttributeName=gsi1sk,AttributeType=S \
    AttributeName=gsi2pk,AttributeType=S \
    AttributeName=gsi2sk,AttributeType=S \
  --key-schema \
    AttributeName=pk,KeyType=HASH \
    AttributeName=sk,KeyType=RANGE \
  --global-secondary-indexes \
    '[
      {
        "IndexName": "email-index",
        "KeySchema": [
          {"AttributeName": "gsi1pk", "KeyType": "HASH"},
          {"AttributeName": "gsi1sk", "KeyType": "RANGE"}
        ],
        "Projection": {"ProjectionType": "ALL"},
        "ProvisionedThroughput": {"ReadCapacityUnits": 5, "WriteCapacityUnits": 5}
      },
      {
        "IndexName": "status-index",
        "KeySchema": [
          {"AttributeName": "gsi2pk", "KeyType": "HASH"},
          {"AttributeName": "gsi2sk", "KeyType": "RANGE"}
        ],
        "Projection": {"ProjectionType": "ALL"},
        "ProvisionedThroughput": {"ReadCapacityUnits": 5, "WriteCapacityUnits": 5}
      }
    ]' \
  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
  --region $REGION 2>/dev/null || echo "Table already exists"

echo "⏰ Enabling TTL..."
aws dynamodb update-time-to-live \
  --endpoint-url $ENDPOINT \
  --table-name $TABLE_NAME \
  --time-to-live-specification "Enabled=true,AttributeName=ttl" \
  --region $REGION 2>/dev/null || echo "TTL already enabled"

echo ""
echo "✅ DynamoDB Local ready!"
echo "   DynamoDB:  $ENDPOINT"
echo "   Admin GUI: http://localhost:8001"
echo "   Table:     $TABLE_NAME"
