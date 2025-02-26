/*
 * Copyright © 2018 A Bunch Tell LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"fmt"
	"github.com/microcosm-cc/bluemonday"
	stripmd "github.com/writeas/go-strip-markdown"
	"github.com/writeas/saturday"
	"github.com/writeas/web-core/stringmanip"
	"github.com/writeas/writefreely/parse"
	"html"
	"html/template"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	blockReg        = regexp.MustCompile("<(ul|ol|blockquote)>\n")
	endBlockReg     = regexp.MustCompile("</([a-z]+)>\n</(ul|ol|blockquote)>")
	youtubeReg      = regexp.MustCompile("(https?://www.youtube.com/embed/[a-zA-Z0-9\\-_]+)(\\?[^\t\n\f\r \"']+)?")
	titleElementReg = regexp.MustCompile("</?h[1-6]>")
	hashtagReg      = regexp.MustCompile(`{{\[\[\|\|([^|]+)\|\|\]\]}}`)
	markeddownReg   = regexp.MustCompile("<p>(.+)</p>")
)

func (p *Post) formatContent(c *Collection, isOwner bool) {
	baseURL := c.CanonicalURL()
	if !isSingleUser {
		baseURL = "/" + c.Alias + "/"
	}
	p.HTMLTitle = template.HTML(applyBasicMarkdown([]byte(p.Title.String)))
	p.HTMLContent = template.HTML(applyMarkdown([]byte(p.Content), baseURL))
	if exc := strings.Index(string(p.Content), "<!--more-->"); exc > -1 {
		p.HTMLExcerpt = template.HTML(applyMarkdown([]byte(p.Content[:exc]), baseURL))
	}
}

func (p *PublicPost) formatContent(isOwner bool) {
	p.Post.formatContent(&p.Collection.Collection, isOwner)
}

func applyMarkdown(data []byte, baseURL string) string {
	return applyMarkdownSpecial(data, false, baseURL)
}

func applyMarkdownSpecial(data []byte, skipNoFollow bool, baseURL string) string {
	mdExtensions := 0 |
		blackfriday.EXTENSION_TABLES |
		blackfriday.EXTENSION_FENCED_CODE |
		blackfriday.EXTENSION_AUTOLINK |
		blackfriday.EXTENSION_STRIKETHROUGH |
		blackfriday.EXTENSION_SPACE_HEADERS |
		blackfriday.EXTENSION_AUTO_HEADER_IDS
	htmlFlags := 0 |
		blackfriday.HTML_USE_SMARTYPANTS |
		blackfriday.HTML_SMARTYPANTS_DASHES

	if baseURL != "" {
		htmlFlags |= blackfriday.HTML_HASHTAGS
	}

	// Generate Markdown
	md := blackfriday.Markdown([]byte(data), blackfriday.HtmlRenderer(htmlFlags, "", ""), mdExtensions)
	if baseURL != "" {
		// Replace special text generated by Markdown parser
		md = []byte(hashtagReg.ReplaceAll(md, []byte("<a href=\""+baseURL+"tag:$1\" class=\"hashtag\"><span>#</span><span class=\"p-category\">$1</span></a>")))
	}
	// Strip out bad HTML
	policy := getSanitizationPolicy()
	policy.RequireNoFollowOnLinks(!skipNoFollow)
	outHTML := string(policy.SanitizeBytes(md))
	// Strip newlines on certain block elements that render with them
	outHTML = blockReg.ReplaceAllString(outHTML, "<$1>")
	outHTML = endBlockReg.ReplaceAllString(outHTML, "</$1></$2>")
	// Remove all query parameters on YouTube embed links
	// TODO: make this more specific. Taking the nuclear approach here to strip ?autoplay=1
	outHTML = youtubeReg.ReplaceAllString(outHTML, "$1")

	return outHTML
}

func applyBasicMarkdown(data []byte) string {
	mdExtensions := 0 |
		blackfriday.EXTENSION_STRIKETHROUGH |
		blackfriday.EXTENSION_SPACE_HEADERS |
		blackfriday.EXTENSION_HEADER_IDS
	htmlFlags := 0 |
		blackfriday.HTML_SKIP_HTML |
		blackfriday.HTML_USE_SMARTYPANTS |
		blackfriday.HTML_SMARTYPANTS_DASHES

	// Generate Markdown
	md := blackfriday.Markdown([]byte(data), blackfriday.HtmlRenderer(htmlFlags, "", ""), mdExtensions)
	// Strip out bad HTML
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class", "id").Globally()
	outHTML := string(policy.SanitizeBytes(md))
	outHTML = markeddownReg.ReplaceAllString(outHTML, "$1")
	outHTML = strings.TrimRightFunc(outHTML, unicode.IsSpace)

	return outHTML
}

func postTitle(content, friendlyId string) string {
	const maxTitleLen = 80

	// Strip HTML tags with bluemonday's StrictPolicy, then unescape the HTML
	// entities added in by sanitizing the content.
	content = html.UnescapeString(bluemonday.StrictPolicy().Sanitize(content))

	content = strings.TrimLeftFunc(stripmd.Strip(content), unicode.IsSpace)
	eol := strings.IndexRune(content, '\n')
	blankLine := strings.Index(content, "\n\n")
	if blankLine != -1 && blankLine <= eol && blankLine <= assumedTitleLen {
		return strings.TrimSpace(content[:blankLine])
	} else if utf8.RuneCountInString(content) <= maxTitleLen {
		return content
	}
	return friendlyId
}

// TODO: fix duplicated code from postTitle. postTitle is a widely used func we
// don't have time to investigate right now.
func friendlyPostTitle(content, friendlyId string) string {
	const maxTitleLen = 80

	// Strip HTML tags with bluemonday's StrictPolicy, then unescape the HTML
	// entities added in by sanitizing the content.
	content = html.UnescapeString(bluemonday.StrictPolicy().Sanitize(content))

	content = strings.TrimLeftFunc(stripmd.Strip(content), unicode.IsSpace)
	eol := strings.IndexRune(content, '\n')
	blankLine := strings.Index(content, "\n\n")
	if blankLine != -1 && blankLine <= eol && blankLine <= assumedTitleLen {
		return strings.TrimSpace(content[:blankLine])
	} else if eol == -1 && utf8.RuneCountInString(content) <= maxTitleLen {
		return content
	}
	title, truncd := parse.TruncToWord(parse.PostLede(content, true), maxTitleLen)
	if truncd {
		title += "..."
	}
	return title
}

func getSanitizationPolicy() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("src", "style").OnElements("iframe", "video", "audio")
	policy.AllowAttrs("src", "type").OnElements("source")
	policy.AllowAttrs("frameborder", "width", "height").Matching(bluemonday.Integer).OnElements("iframe")
	policy.AllowAttrs("allowfullscreen").OnElements("iframe")
	policy.AllowAttrs("controls", "loop", "muted", "autoplay").OnElements("video")
	policy.AllowAttrs("controls", "loop", "muted", "autoplay", "preload").OnElements("audio")
	policy.AllowAttrs("target").OnElements("a")
	policy.AllowAttrs("style", "class", "id").Globally()
	policy.AllowURLSchemes("http", "https", "mailto", "xmpp")
	return policy
}

func sanitizePost(content string) string {
	return strings.Replace(content, "<", "&lt;", -1)
}

// postDescription generates a description based on the given post content,
// title, and post ID. This doesn't consider a V2 post field, `title` when
// choosing what to generate. In case a post has a title, this function will
// fail, and logic should instead be implemented to skip this when there's no
// title, like so:
//    var desc string
//    if title == "" {
//        desc = postDescription(content, title, friendlyId)
//    } else {
//        desc = shortPostDescription(content)
//    }
func postDescription(content, title, friendlyId string) string {
	maxLen := 140

	if content == "" {
		content = "WriteFreely is a painless, simple, federated blogging platform."
	} else {
		fmtStr := "%s"
		truncation := 0
		if utf8.RuneCountInString(content) > maxLen {
			// Post is longer than the max description, so let's show a better description
			fmtStr = "%s..."
			truncation = 3
		}

		if title == friendlyId {
			// No specific title was found; simply truncate the post, starting at the beginning
			content = fmt.Sprintf(fmtStr, strings.Replace(stringmanip.Substring(content, 0, maxLen-truncation), "\n", " ", -1))
		} else {
			// There was a title, so return a real description
			blankLine := strings.Index(content, "\n\n")
			if blankLine < 0 {
				blankLine = 0
			}
			truncd := stringmanip.Substring(content, blankLine, blankLine+maxLen-truncation)
			contentNoNL := strings.Replace(truncd, "\n", " ", -1)
			content = strings.TrimSpace(fmt.Sprintf(fmtStr, contentNoNL))
		}
	}

	return content
}

func shortPostDescription(content string) string {
	maxLen := 140
	fmtStr := "%s"
	truncation := 0
	if utf8.RuneCountInString(content) > maxLen {
		// Post is longer than the max description, so let's show a better description
		fmtStr = "%s..."
		truncation = 3
	}
	return strings.TrimSpace(fmt.Sprintf(fmtStr, strings.Replace(stringmanip.Substring(content, 0, maxLen-truncation), "\n", " ", -1)))
}
