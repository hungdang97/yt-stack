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
public class CommentsPageDTO {
    private List<CommentDTO> comments;
    private String nextPageToken;
    private Boolean hasNextPage;
    private Boolean disabled;
}
