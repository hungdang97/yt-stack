# Multi-stage build for YouTube API
# Stage 1: Build stage
# Using amazoncorretto for better ARM64 (Apple Silicon) support
FROM amazoncorretto:17-alpine AS builder

WORKDIR /app

# Copy Gradle wrapper and configuration files
COPY gradlew .
COPY gradle gradle
COPY build.gradle .
COPY settings.gradle .

# Make gradlew executable
RUN chmod +x gradlew

# Download dependencies (cached layer)
RUN ./gradlew dependencies --no-daemon || true

# Copy source code
COPY src src

# Build the application (skip tests for faster build)
RUN ./gradlew clean bootJar -x test --no-daemon

# Stage 2: Runtime stage
FROM amazoncorretto:17-alpine

# Install curl for health checks (optional)
RUN apk add --no-cache curl

# Create non-root user for security
RUN addgroup -g 1001 appuser && \
  adduser -D -u 1001 -G appuser appuser

WORKDIR /app

# Copy the built JAR from builder stage
COPY --from=builder /app/build/libs/*.jar app.jar

# Change ownership to non-root user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose application port
EXPOSE 8100

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
  CMD curl -f http://localhost:8100/api/youtube/health || exit 1

# JVM optimization flags
ENV JAVA_OPTS="-XX:+UseContainerSupport -XX:MaxRAMPercentage=75.0 -XX:InitialRAMPercentage=50.0"

# Run the application
ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS -jar app.jar"]
