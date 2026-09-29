# Сборочный контейнер на базе официального репозитория Alt Linux p10 (x86_64)
ARG BASE_IMAGE=alt:p10
FROM --platform=linux/amd64 ${BASE_IMAGE}

# Установка компилятора Go и системных зависимостей графического стека Alt Linux
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

# Сборка бинарника с CGO под графическую систему Alt Linux x86_64 (glibc 2.32)
CMD ["go", "build", "-mod=vendor", "-tags", "desktop,production", "-ldflags", "-s -w", "-o", "bin/depersonalizer-altlinux-amd64", "./cmd/app"]
