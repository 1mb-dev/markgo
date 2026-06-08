package new

import (
	"fmt"
	"strings"
	"time"
)

// ArticleTemplate defines a template for creating articles
type ArticleTemplate struct {
	Name        string
	Description string
	Generator   func(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string
}

// GetAvailableTemplates returns all available article templates
func GetAvailableTemplates() map[string]ArticleTemplate {
	return map[string]ArticleTemplate{
		"default": {
			Name:        "Default Article",
			Description: "Standard blog article with intro, content, and conclusion",
			Generator:   generateDefaultArticle,
		},
		"tutorial": {
			Name:        "Tutorial",
			Description: "Step-by-step tutorial with prerequisites and examples",
			Generator:   generateTutorialArticle,
		},
		"review": {
			Name:        "Product/Book Review",
			Description: "Structured review with pros, cons, and rating",
			Generator:   generateReviewArticle,
		},
		"news": {
			Name:        "News Article",
			Description: "News article with summary and key points",
			Generator:   generateNewsArticle,
		},
		"howto": {
			Name:        "How-To Guide",
			Description: "Problem-solving guide with clear steps",
			Generator:   generateHowToArticle,
		},
		"opinion": {
			Name:        "Opinion Piece",
			Description: "Editorial or opinion article with arguments",
			Generator:   generateOpinionArticle,
		},
		"listicle": {
			Name:        "List Article",
			Description: "Numbered or bulleted list article",
			Generator:   generateListicleArticle,
		},
		"interview": {
			Name:        "Interview",
			Description: "Q&A format interview article",
			Generator:   generateInterviewArticle,
		},
		"minimal": {
			Name:        "Minimal",
			Description: "Minimal template with just frontmatter and title",
			Generator:   generateMinimalArticle,
		},
	}
}

// generateDefaultArticle creates the standard article template
func generateDefaultArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

Content goes here...

## Introduction

Write your introduction here.

## Main Content

Add your main content sections here.

## Conclusion

Wrap up your article with a conclusion.

---

*Written by {{.Author}} on {{.FormattedDate}}*`)
}

// generateTutorialArticle creates a tutorial template
func generateTutorialArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

A comprehensive tutorial on {{.Title}}.

## Prerequisites

Before starting this tutorial, you should have:

- [ ] Prerequisite 1
- [ ] Prerequisite 2
- [ ] Prerequisite 3

## What You'll Learn

By the end of this tutorial, you will:

- Learn concept A
- Understand technique B
- Be able to implement C

## Step 1: Getting Started

Explain the first step here.

`+"```"+`bash
# Example command
echo "Hello, World!"
`+"```"+`

## Step 2: Next Steps

Continue with detailed steps.

## Step 3: Advanced Topics

Cover more complex concepts.

## Troubleshooting

Common issues and solutions:

**Issue**: Problem description
**Solution**: How to fix it

## Conclusion

Summary of what was covered and next steps.

## Further Reading

- [Resource 1](https://example.com)
- [Resource 2](https://example.com)

---

*Tutorial by {{.Author}} - {{.FormattedDate}}*`)
}

// generateReviewArticle creates a review template
func generateReviewArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

A detailed review of [Product/Book/Service Name].

## Quick Summary

**Rating**: ⭐⭐⭐⭐⭐ (5/5)
**Price**: $XX.XX
**Best For**: Target audience
**Bottom Line**: Brief verdict

## Overview

Brief description of what you're reviewing and why.

## What's Good

### Pros
- ✅ Strength 1
- ✅ Strength 2
- ✅ Strength 3

## What's Not So Good

### Cons
- ❌ Weakness 1
- ❌ Weakness 2
- ❌ Weakness 3

## Detailed Analysis

### Feature 1
Detailed discussion of this feature.

### Feature 2
Analysis of another important aspect.

### Value for Money
Discussion of pricing and value proposition.

## Alternatives

Brief mention of competing products/services.

## Final Verdict

**Would I recommend it?** Yes/No, and why.

**Rating Breakdown:**
- Quality: X/5
- Value: X/5
- Usability: X/5
- Support: X/5

---

*Review by {{.Author}} - {{.FormattedDate}}*`)
}

// generateNewsArticle creates a news article template
func generateNewsArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

**Location, Date** - Lead paragraph summarizing the key points of the story.

## Key Points

- 📰 Main point 1
- 📈 Main point 2
- 💡 Main point 3

## Background

Context and background information about the story.

## Current Developments

What's happening now and recent updates.

## Impact

How this affects readers, industry, or society.

## What's Next

Expected future developments or timeline.

## Quotes

> "Relevant quote from key figure"
> — **Name, Title**

## Related Stories

- [Related Article 1](#)
- [Related Article 2](#)

---

*Reported by {{.Author}} - {{.FormattedDate}}*`)
}

// generateHowToArticle creates a how-to guide template
func generateHowToArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

Learn how to accomplish this task step by step.

## Problem

Describe the problem this guide solves.

## Solution Overview

Brief explanation of the approach.

## What You'll Need

### Tools
- Tool 1
- Tool 2

### Materials/Requirements
- Requirement 1
- Requirement 2

## Step-by-Step Guide

### Step 1: Preparation
Detailed instructions for the first step.

### Step 2: Implementation
Instructions for the main implementation.

### Step 3: Verification
How to check that everything worked correctly.

## Tips and Best Practices

- 💡 **Tip 1**: Helpful advice
- ⚠️ **Warning**: Important caution
- 🔧 **Pro Tip**: Advanced technique

## Common Mistakes to Avoid

1. **Mistake 1**: Why it's wrong and how to avoid it
2. **Mistake 2**: Common pitfall and solution

## Troubleshooting

**Problem**: Issue description
**Solution**: How to fix it

## Conclusion

Summary and final thoughts.

---

*Guide by {{.Author}} - {{.FormattedDate}}*`)
}

// generateOpinionArticle creates an opinion piece template
func generateOpinionArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

My thoughts on [topic] and why it matters.

## The Issue

Description of the topic or situation being discussed.

## My Position

Clear statement of your stance or opinion.

## Supporting Arguments

### Argument 1
Evidence and reasoning supporting your position.

### Argument 2
Additional supporting evidence.

### Argument 3
Further support for your viewpoint.

## Counterarguments

### Common Objection 1
Acknowledgment of opposing views and your response.

### Common Objection 2
Another counterargument and your rebuttal.

## Why This Matters

Explanation of broader implications and importance.

## What Should Happen Next

Your recommendations or call to action.

## Conclusion

Restatement of your position and final thoughts.

---

*Opinion by {{.Author}} - {{.FormattedDate}}*

*Disclaimer: These views are my own and do not necessarily represent the views of any organization.*`)
}

// generateListicleArticle creates a list-style article template
func generateListicleArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

A curated list of [items] that will help you [benefit].

## Introduction

Brief introduction to the list and its purpose.

## The List

### 1. First Item
**Why it matters**: Explanation of significance
**Key features**: What makes it special
**Best for**: Who should use this

### 2. Second Item
**Why it matters**: Explanation of significance
**Key features**: What makes it special
**Best for**: Who should use this

### 3. Third Item
**Why it matters**: Explanation of significance
**Key features**: What makes it special
**Best for**: Who should use this

### 4. Fourth Item
**Why it matters**: Explanation of significance
**Key features**: What makes it special
**Best for**: Who should use this

### 5. Fifth Item
**Why it matters**: Explanation of significance
**Key features**: What makes it special
**Best for**: Who should use this

## Bonus Items

### Honorable Mention 1
Brief description of additional item.

### Honorable Mention 2
Brief description of another additional item.

## How to Choose

Guidelines for selecting the best option for your needs.

## Conclusion

Summary of the list and final recommendations.

---

*Curated by {{.Author}} - {{.FormattedDate}}*`)
}

// generateInterviewArticle creates an interview template
func generateInterviewArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

An interview with [Interviewee Name], [Title/Role].

## About the Interviewee

Brief background about the person being interviewed.

**Name**: [Full Name]
**Role**: [Current Position]
**Background**: [Relevant Experience]
**Contact**: [Website/Social Media]

## The Interview

### Q: First question about their background?

**A**: Their response here.

### Q: Question about their work/expertise?

**A**: Detailed response.

### Q: What challenges do you face in your field?

**A**: Discussion of challenges.

### Q: What advice would you give to newcomers?

**A**: Advice and recommendations.

### Q: What's next for you/your industry?

**A**: Future outlook and predictions.

### Q: Any final thoughts for our readers?

**A**: Closing remarks.

## Key Takeaways

- 💡 **Insight 1**: Important point from the interview
- 📈 **Insight 2**: Another key takeaway
- 🚀 **Insight 3**: Future-focused insight

## Connect with [Interviewee Name]

- **Website**: [URL]
- **Twitter**: [@handle]
- **LinkedIn**: [Profile URL]

---

*Interview conducted by {{.Author}} - {{.FormattedDate}}*`)
}

// generateMinimalArticle creates a minimal template
func generateMinimalArticle(title, description, tagsStr, category, author string, isDraft, isFeatured bool) string {
	return generateArticleWithTemplate(title, description, tagsStr, category, author, isDraft, isFeatured, `# {{.Title}}

Start writing your content here...`)
}

// injectSlugFrontmatter inserts a `slug:` line into the generated article's
// frontmatter so the served URL is the resolved slug rather than one the
// repository re-derives from the title at load time (repository.go falls back
// to slug.Generate(title) only when frontmatter carries no slug). The compose
// path persists `slug:` the same way. The slug is charset-validated by
// slug.Validate ([a-z0-9-] only), so %q cannot introduce YAML metacharacters.
func injectSlugFrontmatter(content, slug string) string {
	const open = "---\n"
	if !strings.HasPrefix(content, open) {
		return content
	}
	return open + fmt.Sprintf("slug: %q\n", slug) + content[len(open):]
}

// generateArticleWithTemplate is a helper function that creates the full article
func generateArticleWithTemplate(title, description, tagsStr, category, author string, isDraft, isFeatured bool, contentTemplate string) string {
	now := time.Now()

	// Format tags as YAML array
	tagList := strings.Split(tagsStr, ",")
	for i, tag := range tagList {
		tagList[i] = strings.TrimSpace(tag)
	}
	formattedTags := strings.Join(tagList, ", ")

	// Create template data
	templateData := struct {
		Title         string
		Author        string
		FormattedDate string
		Date          string
	}{
		Title:         title,
		Author:        author,
		FormattedDate: now.Format("January 2, 2006"),
		Date:          now.Format(time.RFC3339),
	}

	// Replace template variables
	content := contentTemplate
	content = strings.ReplaceAll(content, "{{.Title}}", templateData.Title)
	content = strings.ReplaceAll(content, "{{.Author}}", templateData.Author)
	content = strings.ReplaceAll(content, "{{.FormattedDate}}", templateData.FormattedDate)

	// Generate frontmatter — sanitize values interpolated into double-quoted YAML scalars
	safeTitle := SanitizeForYAML(title)
	safeDescription := SanitizeForYAML(description)
	safeAuthor := SanitizeForYAML(author)

	frontmatter := fmt.Sprintf(`---
title: "%s"
description: "%s"
date: %s
tags: [%s]
categories: [%s]
featured: %v
draft: %v
author: "%s"
---

`, safeTitle, safeDescription, templateData.Date, formattedTags, category, isFeatured, isDraft, safeAuthor)

	return frontmatter + content
}
