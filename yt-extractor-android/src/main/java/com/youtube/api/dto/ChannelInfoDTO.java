package com.youtube.api.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ChannelInfoDTO {
    private String id;
    private String name;
    private String url;
    private String description;
    private List<String> avatarUrl;
    private List<String> bannerUrl;
    private Long subscriberCount;
    private Long videoCount;
    private Boolean verified;
    private List<ChannelTabDTO> tabs;
}
