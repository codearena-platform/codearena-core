$PWD_PATH = Get-Location
$OUT_DIR = "docs/api"

# Auto-detect container engine
$ContainerEngine = "podman"
if (!(Get-Command podman -ErrorAction SilentlyContinue)) {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        $ContainerEngine = "docker"
    }
    else {
        Write-Error "Error: Neither 'podman' nor 'docker' found in PATH."
        exit 1
    }
}

Write-Host "Using container engine: $ContainerEngine" -ForegroundColor Cyan

# Ensure output dir exists
if (!(Test-Path $OUT_DIR)) {
    New-Item -ItemType Directory -Path $OUT_DIR | Out-Null
}

# Check if generator image exists
$ImageId = & $ContainerEngine images -q codearena-gen
if (-not $ImageId) {
    Write-Host "Generator image 'codearena-gen' not found. Building..." -ForegroundColor Yellow
    & $ContainerEngine build -t codearena-gen -f scripts/Dockerfile.gen scripts
}

Write-Host "Running protoc via $ContainerEngine..." -ForegroundColor Cyan
Write-Host "Mounting: $PWD_PATH -> /workspace"

# Run Container
# We set working directory to the proto folder so imports resolve correctly
# We output OpenAPI to ../../../docs/api
# We output Go code to ../../../pkg/api/v1 (relative to api/proto/v1)
& $ContainerEngine run --rm `
    -v "${PWD_PATH}:/workspace" `
    -w /workspace/api/proto/v1 `
    codearena-gen `
    --proto_path=. `
    --go_out=../../../pkg/api/v1 --go_opt=paths=source_relative `
    --go-grpc_out=../../../pkg/api/v1 --go-grpc_opt=paths=source_relative `
    --openapi_out=../../../docs/api `
    arena.proto bot_api.proto runtime.proto

Write-Host "Done!" -ForegroundColor Green
Get-ChildItem $OUT_DIR
