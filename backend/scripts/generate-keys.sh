#!/bin/sh

# Generate JWT keys for Podland backend
# This script creates RSA private and public keys for JWT token signing

KEY_DIR="$(dirname "$0")/.."
PRIVATE_KEY_PATH="${KEY_DIR}/jwt-private.pem"
PUBLIC_KEY_PATH="${KEY_DIR}/jwt-public.pem"

echo "Generating JWT keys..."

# Generate private key
openssl genrsa -out "$PRIVATE_KEY_PATH" 4096
if [ $? -ne 0 ]; then
    echo "Failed to generate private key"
    exit 1
fi

# Extract public key from private key
openssl rsa -in "$PRIVATE_KEY_PATH" -pubout -out "$PUBLIC_KEY_PATH"
if [ $? -ne 0 ]; then
    echo "Failed to generate public key"
    exit 1
fi

# Set appropriate permissions
chmod 600 "$PRIVATE_KEY_PATH"
chmod 644 "$PUBLIC_KEY_PATH"

echo "JWT keys generated successfully:"
echo "  Private key: $PRIVATE_KEY_PATH"
echo "  Public key:  $PUBLIC_KEY_PATH"
echo ""
echo "Make sure to keep the private key secure!"
echo "Add these files to .gitignore"
