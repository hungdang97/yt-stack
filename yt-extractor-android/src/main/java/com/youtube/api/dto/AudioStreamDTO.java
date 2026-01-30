package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class AudioStreamDTO {
    private String url;
    private String quality;
    private String format;
    private String mimeType;
    private Long bitrate;
    private Long fileSize;
    private String codec;
    private String audioTrackId;
    private String audioTrackType;
    private Boolean isOriginal;
}
