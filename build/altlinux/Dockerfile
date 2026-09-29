# Сборочный контейнер на базе официального репозитория Alt Linux p10
ARG BASE_IMAGE=alt-builder:p10
FROM ${BASE_IMAGE}

# Обновление и установка компилятора Go и системных зависимостей Alt Linux
RUN apt-get update && apt-get install -y \
    libwebkit2gtk-devel \
    libgtk+3-devel \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .

# Сборка бинарника с CGO под графическую систему Alt Linux (glibc 2.32)
CMD ["go", "build", "-mod=vendor", "-tags", "desktop,production", "-ldflags", "-s -w", "-o", "bin/depersonalizer-altlinux-amd64", "./cmd/app"]
