# ScoutPulse Smoke Test
Write-Host "Starting Smoke Test..." -ForegroundColor Cyan

# 1. Validate Docker Compose File
Write-Host "Checking docker compose.yml syntax..."
docker compose config
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Invalid docker compose.yml" -ForegroundColor Red
    exit 1
}

# 2. Build the images
Write-Host "Building images..."
docker compose build
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Build failed" -ForegroundColor Red
    exit 1
}

# 3. Start the stack
Write-Host "Starting stack..."
docker compose up -d
if ($LASTEXITCODE -ne 0) {
    Write-Host "Error: Could not start stack" -ForegroundColor Red
    exit 1
}

# 4. Wait a few seconds for DBs to initialize
Write-Host "Waiting for services to stabilize..."
Start-Sleep -Seconds 5

# 5. Check if containers are running
$containers = docker compose ps --format json | ConvertFrom-Json
$runningCount = ($containers | Where-Object { $_.State -eq "running" }).Count

Write-Host "Running containers: $runningCount"

if ($runningCount -ge 2) {
    Write-Host "SUCCESS: Stack is running correctly!" -ForegroundColor Green
} else {
    Write-Host "FAILURE: Some services failed to start." -ForegroundColor Red
    docker compose ps
    exit 1
}

# 6. Cleanup
Write-Host "Cleaning up..."
docker compose down
Write-Host "Smoke test complete."
