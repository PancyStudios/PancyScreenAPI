#!/bin/bash

# Comprobar arquitectura pasada por parámetro
ARCH=$1

if [ -z "$ARCH" ]; then
  echo "Uso: ./build.sh <amd64|arm64>"
  exit 1
fi

VERSION="1.0.0"
BUILD_TIME=$(date "+%Y-%m-%d %H:%M:%S")

# Definir la extensión/nombre basado en la arquitectura
if [ "$ARCH" == "amd64" ]; then
  OUTPUT_NAME="PancyScreenAPI.x86_64"
elif [ "$ARCH" == "arm64" ]; then
  OUTPUT_NAME="PancyScreenAPI.aarch64"
else
  echo "Arquitectura no soportada. Usa amd64 o arm64."
  exit 1
fi

echo "========================================="
echo "Compilando PancyScreenShots"
echo "Arquitectura: $ARCH"
echo "Versión: $VERSION"
echo "Tiempo de build: $BUILD_TIME"
echo "========================================="

env GOOS=linux GOARCH=$ARCH go build -ldflags "-s -w -X 'main.Version=$VERSION' -X 'main.BuildTime=$BUILD_TIME'" -o $OUTPUT_NAME main.go

if [ $? -eq 0 ]; then
  echo "✅ Build completado exitosamente: $OUTPUT_NAME"
else
  echo "❌ Error al compilar."
  exit 1
fi
