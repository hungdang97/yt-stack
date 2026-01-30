package com.youtube.api.exception;

import org.schabi.newpipe.extractor.exceptions.ExtractionException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ControllerAdvice;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.context.request.WebRequest;

import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

@ControllerAdvice
public class GlobalExceptionHandler {

    @ExceptionHandler(ExtractionException.class)
    public ResponseEntity<Map<String, Object>> handleExtractionException(
            ExtractionException ex, WebRequest request) {
        Map<String, Object> response = new HashMap<>();
        response.put("error", "Extraction failed");
        response.put("message", ex.getMessage());
        response.put("status", HttpStatus.BAD_REQUEST.value());

        return ResponseEntity.status(HttpStatus.BAD_REQUEST).body(response);
    }

    @ExceptionHandler(IOException.class)
    public ResponseEntity<Map<String, Object>> handleIOException(
            IOException ex, WebRequest request) {
        Map<String, Object> response = new HashMap<>();
        response.put("error", "Network error");
        response.put("message", "Failed to connect to YouTube");
        response.put("status", HttpStatus.SERVICE_UNAVAILABLE.value());

        return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(response);
    }

    @ExceptionHandler(Exception.class)
    public ResponseEntity<Map<String, Object>> handleGlobalException(
            Exception ex, WebRequest request) {
        Map<String, Object> response = new HashMap<>();
        response.put("error", "Internal server error");
        response.put("message", ex.getMessage());
        response.put("status", HttpStatus.INTERNAL_SERVER_ERROR.value());

        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body(response);
    }
}
