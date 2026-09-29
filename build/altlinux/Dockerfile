# Сборочный контейнер на базе официального репозитория Alt Linux p10
FROM registry.altlinux.org/alt/p10:latest

# Обновление и установка компилятора Go и системных зависимостей Alt Linux
RUN apt-get update && apt-get install -y \
    golang \
    git \
    make \
    rpm-build \
    gcc \
    libwebkit2gtk-devel \
    libgtk+3-devel \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .

# Сборка бинарника с CGO под графическую систему Alt Linux
CMD ["go", "build", "-tags", "desktop", "-ldflags", "-s -w", "-o", "bin/depersonalizer-altlinux-amd64", "./cmd/app"]
