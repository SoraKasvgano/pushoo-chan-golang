@echo off
REM Build script for pushoo-chan Go version (Windows)
REM Compiles binaries for multiple platforms
REM Run this script from the gover directory

echo ========================================
echo Building pushoo-chan-gover for multiple platforms
echo ========================================

REM Create dist directory
if not exist "dist" mkdir dist

REM Set Go environment
set CGO_ENABLED=0
set GO111MODULE=on

echo.
echo [1/5] Building for Linux AMD64...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o dist/pushoo-chan-gover-linux-amd64 .
if %errorlevel% neq 0 (
    echo ERROR: Failed to build for Linux AMD64
    exit /b 1
)
echo ✓ Linux AMD64 build complete

echo.
echo [2/5] Building for Linux ARM64 (ARMv8)...
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-s -w" -o dist/pushoo-chan-gover-linux-arm64 .
if %errorlevel% neq 0 (
    echo ERROR: Failed to build for Linux ARM64
    exit /b 1
)
echo ✓ Linux ARM64 build complete

echo.
echo [3/5] Building for Linux ARM (ARMv7)...
set GOOS=linux
set GOARCH=arm
set GOARM=7
go build -ldflags="-s -w" -o dist/pushoo-chan-gover-linux-armv7 .
if %errorlevel% neq 0 (
    echo ERROR: Failed to build for Linux ARMv7
    exit /b 1
)
echo ✓ Linux ARMv7 build complete

echo.
echo [4/5] Building for Windows AMD64...
set GOOS=windows
set GOARCH=amd64
set GOARM=
go build -ldflags="-s -w" -o dist/pushoo-chan-gover-windows-amd64.exe .
if %errorlevel% neq 0 (
    echo ERROR: Failed to build for Windows AMD64
    exit /b 1
)
echo ✓ Windows AMD64 build complete

echo.
echo [5/5] Building for current platform (local test)...
set GOOS=
set GOARCH=
set GOARM=
go build -o dist/pushoo-chan-gover-local.exe .
if %errorlevel% neq 0 (
    echo ERROR: Failed to build for local platform
    exit /b 1
)
echo ✓ Local build complete

echo.
echo ========================================
echo Build Summary
echo ========================================
dir dist /b
echo.
echo All builds completed successfully!
echo Binaries are located in the 'dist' folder.
echo ========================================

pause
