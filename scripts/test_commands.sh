#!/bin/bash
# Quick test commands for the KV store
# Run these in a separate terminal while the server is running

echo "=== Testing Put ==="
grpcurl -plaintext -d '{
  "key": "hello",
  "value": "SGVsbG8gV29ybGQ=",
  "client_id": "test-client",
  "request_id": "req1"
}' localhost:50051 kvstore.KVStore/Put

echo -e "\n=== Testing Get ==="
grpcurl -plaintext -d '{
  "key": "hello",
  "client_id": "test-client",
  "request_id": "req2"
}' localhost:50051 kvstore.KVStore/Get

echo -e "\n=== Testing Delete ==="
grpcurl -plaintext -d '{
  "key": "hello",
  "client_id": "test-client",
  "request_id": "req3"
}' localhost:50051 kvstore.KVStore/Delete

echo -e "\n=== Verifying Delete (should return NOT_FOUND) ==="
grpcurl -plaintext -d '{
  "key": "hello",
  "client_id": "test-client",
  "request_id": "req4"
}' localhost:50051 kvstore.KVStore/Get

