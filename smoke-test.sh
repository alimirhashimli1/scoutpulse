#!/bin/bash

# ScoutPulse Smoke Test (Linux/WSL version)
echo -e "\e[36mStarting Smoke Test...\e[0m"

# 1. Validate Docker Compose File
echo "Checking docker-compose.yml syntax..."
docker compose config > /dev/null
if [ $? -ne 0 ]; then
    echo -e "\e[31mError: Invalid docker-compose.yml\e[0m"
    exit 1
fi

# 2. Build the images
echo "Building images..."
docker compose build
if [ $? -ne 0 ]; then
    echo -e "\e[31mError: Build failed\e[0m"
    exit 1
fi

# 3. Start the stack
echo "Starting stack..."
docker compose up -d
if [ $? -ne 0 ]; then
    echo -e "\e[31mError: Could not start stack\e[0m"
    exit 1
fi

# 4. Wait for the stack to become healthy.
#
# The gateway now waits on `service_healthy` for both services rather than on
# their containers merely existing, so readiness is sequenced: migrations, then
# the services, then their healthchecks passing, then the gateway. That takes
# longer than a bare `up -d` did.
echo "Waiting for services to stabilize (40s)..."
sleep 40

# 5. Check if containers are running
# We expect 4 containers based on your previous output
running_count=$(docker compose ps --format json | grep -c '"State":"running"')

echo "Running containers: $running_count"

if [ "$running_count" -ge 4 ]; then
    echo -e "\e[32mSUCCESS: Stack is running correctly!\e[0m"
else
    echo -e "\e[31mFAILURE: Some services failed to start.\e[0m"
    docker compose ps
    exit 1
fi

# 6. Cleanup (Optional: uncomment if you want it to shut down automatically)
# echo "Cleaning up..."
# docker compose down
echo "Smoke test complete."
