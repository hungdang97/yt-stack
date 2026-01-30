package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class VideoStreamDTO {
    private String url;
    private String quality;
    private String format;
    private String mimeType;
    private Long bitrate;
    private Long fileSize;
    private String codec;
    private Integer width;
    private Integer height;
    private Integer fps;
    private Boolean videoOnly; // true = video-only stream (need separate audio), false = muxed (video+audio)
}
