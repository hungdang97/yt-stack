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
public class SearchResultDTO {
    private List<SearchItemDTO> items;
    private String nextPageToken;
    private Boolean hasNextPage;
}
