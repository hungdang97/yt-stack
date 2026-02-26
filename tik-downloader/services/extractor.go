package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"tik-downloader/config"
	"tik-downloader/models"
)

// Default TikTok cookie for API extraction
const defaultCookie = "_ttp=2hqtHChFe7UTighTpqgKHHA1iSp; uid_tt_ss=38a4b5ae41a9360b3d04a610cc8b06830a0a45b4556798c29169fd69d0c9c6ee; sessionid_ss=6b6f7985871b49380232a4376f4d3755; tta_attr_id_mirror=0.1761013677.7563496152593137680; passport_csrf_token=109af37c1be17247496b5d7ba0bc0876; tt_chain_token=bapr1TRV9NPhvWDkAWJ3LQ==; tiktok_webapp_theme_source=auto; delay_guest_mode_vid=5; uid_tt=38a4b5ae41a9360b3d04a610cc8b06830a0a45b4556798c29169fd69d0c9c6ee; sid_tt=6b6f7985871b49380232a4376f4d3755; sessionid=6b6f7985871b49380232a4376f4d3755; sid_guard=6b6f7985871b49380232a4376f4d3755%7C1772008103%7C15552000%7CMon%2C+24-Aug-2026+08%3A28%3A23+GMT; tt_session_tlb_tag=sttt%7C4%7Ca295hYcbSTgCMqQ3b003Vf_________u6a-Djl1zeRZIo8gIgYMm-D266JfvsK_M3Ul-Sg0Y-FY%3D; sid_ucp_v1=1.0.1-KDU5YjgxNDAyMjIzNjZjYjMwZjM2Y2YzMzFhZTA2NmRhMWY5YjM2NTcKGQiHiJW2nOmg02UQp-X6zAYYsws4CEASSAQQAxoDc2cxIiA2YjZmNzk4NTg3MWI0OTM4MDIzMmE0Mzc2ZjRkMzc1NTJOCiAVRAOiTtEH1fQzJaFrWDYQO8OdUzpulp45ypQ0_dfWjhIgavLs8eeqlJr7QA71_El8WdE83ZFUTX5elInv3ezyqXQYASIGdGlrdG9r; ssid_ucp_v1=1.0.1-KDU5YjgxNDAyMjIzNjZjYjMwZjM2Y2YzMzFhZTA2NmRhMWY5YjM2NTcKGQiHiJW2nOmg02UQp-X6zAYYsws4CEASSAQQAxoDc2cxIiA2YjZmNzk4NTg3MWI0OTM4MDIzMmE0Mzc2ZjRkMzc1NTJOCiAVRAOiTtEH1fQzJaFrWDYQO8OdUzpulp45ypQ0_dfWjhIgavLs8eeqlJr7QA71_El8WdE83ZFUTX5elInv3ezyqXQYASIGdGlrdG9r; store-idc=alisg; store-country-code=vn; store-country-code-src=uid; tt-target-idc=alisg; tt-target-idc-sign=AnOCQcu8CF6ChOkJ3FOehXT2DihA4ZT9dnKIjepnmNRB5Qz6d7mMQvqQzbSoQFgtH2tvpN-R0dVvtwJIU7x4xCQKVaQYoUvcKPWV6NECx25Ba4E8RE_SM0vRQJNFU9pgDHiy3JXx4OcMtX29ElqmkTZgeVsmmpslBwcFANV7C1TxMUrkLmie8PYOHBBiuo3BVqELdOnbQroBv5SUhbqQWFY0wrtDzLESbEijFgiTAtpNIExOgeniKbSG57pMetG0o9Z4Rxw4jlNZkB205tYArtIIcr9mYBipMBJYaJArrccyMsgsk5KoiKRKspi2MVVfeR4j-mg0HJUhd3TdYIJgZSLhfLKpfYwp_T0n_sVho1-N6eheuHp_3UE0TO2kqZDXwHlC5SEZlv51CWHSfxJ03YQ5x2yckSsVn7ripJ5cz52JAdEea_Kf_ArFLZyiwt7JcB1JppcMCjXL1KmFfLWvcGATTNl0A24Sbnajb-OokehzZTM4GbKWy7wJy2srsDxX; tt_ticket_guard_has_set_public_key=1; tt_csrf_token=byDvtQzK-oVU6u0ap9ODaWUZJpFEA-JV-ztQ; tiktok_webapp_theme=light; passport_fe_beating_status=true; ttwid=1%7CzVCqNLWhRD6iKADRRQVUOmHga28_7fEy-cQYGlgvtFE%7C1772068521%7Cf7ec10b70a45615a3162bd4b7cf898b560039bbd7f5725cc5df357318c5679c1; s_v_web_id=verify_mm2ruec8_xSLkH4pQ_2xrW_4ScQ_9YnH_YsSslW1GPEDa; perf_feed_cache={%22expireTimestamp%22:1772240400000%2C%22itemIds%22:[%227593212542835641618%22%2C%227610387867990297864%22%2C%227587602166114766097%22]}; store-country-sign=MEIEDIPTbQP5MW_X7av0uwQg6xmqb-r7jP8u--LL8UN6TTflVO-HURrB9DCTZ0o-igEEEHFrJMJS7W6QMceZR2W8CW8; msToken=MOl4FTrX27v8KTbUkP5ZKLoTOt-HqgPytFQVBd_c2pFgA_WR7CZc0dYC7nTCoJLgCxj41nxvdZtmSpx71obYJjqqD_KSDlx3bx6pAeAw3xM0u3608V4HT19eVshzSMBy0b5t9H6VK8TkUs7BsBSVFEmA; msToken=7ln0s1AzVR5BDjwwa37qBHKCYIqu4IjIWQCNr38qLan0fUseIl1L4a5Ge9uNApQMW0kHl1iVZX8kdg7w3SD5vpPDAZUjO550LieFhx68QWCAcX49me0cgshZYmBfwR4Hgz4JB7W_Sbg7SPcmsosOjrmo; odin_tt=2ad1f625cc2f5d7c92300a00cba49fe6d5b7ad3ee976f4beaec5b95f97625628df6967af37ae916f0c74f1ea1f73036c10f4db05b1ca0b998d4310fff662eb6cfb7f90387e9f9a8bb7c5869cc11725e8;"

// Extract calls tik-extractor API and returns the full response
func Extract(videoID string) (*models.TikExtractResponse, error) {
	reqBody := models.TikExtractRequest{
		DetailID: videoID,
		Cookie:   defaultCookie,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := config.TikExtractorURL + "/tiktok/detail"
	fmt.Printf("[%s] Extracting from %s\n", videoID, url)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := config.ExtractClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extractor API error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extractor API returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse directly into typed struct
	var extractResp models.TikExtractResponse
	if err := json.Unmarshal(respBody, &extractResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	video := &extractResp.Data
	if video.Downloads == "" && video.MusicURL == "" {
		return nil, fmt.Errorf("no download URLs in response (message: %s)", extractResp.Message)
	}

	// Ensure cookie is set for CDN download
	if extractResp.Params.Cookie == "" {
		extractResp.Params.Cookie = defaultCookie
	}

	fmt.Printf("[%s] ✓ Extracted: %s (duration=%s, downloads=%v, music=%v)\n",
		videoID, video.Desc, video.Duration,
		video.Downloads != "", video.MusicURL != "")

	return &extractResp, nil
}

// ParseDuration converts "00:00:30" format to seconds
func ParseDuration(duration string) float64 {
	parts := strings.Split(duration, ":")
	if len(parts) != 3 {
		return 0
	}
	hours, _ := strconv.ParseFloat(parts[0], 64)
	minutes, _ := strconv.ParseFloat(parts[1], 64)
	seconds, _ := strconv.ParseFloat(parts[2], 64)
	return hours*3600 + minutes*60 + seconds
}
