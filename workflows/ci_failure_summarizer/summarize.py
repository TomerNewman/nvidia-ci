#!/usr/bin/env python3
"""
CI Failure Summarizer - AI-powered TL;DR for Prow job failures.

Uses Groq (free tier, no credit card required) to generate concise summaries of CI failures.
Reuses the prow-analyzer GCS client for fetching logs.
"""

import os
import sys
import re
import urllib.parse
from pathlib import Path

import requests
from groq import Groq

# Add parent directory to path for common utils
sys.path.insert(0, str(Path(__file__).parent.parent))
from common.utils import get_logger

logger = get_logger(__name__)

# Configuration
GCS_BUCKET = "test-platform-results"
DEFAULT_ORG_REPO = "rh-ecosystem-edge_nvidia-ci"

SYSTEM_PROMPT = """You are a CI failure analyst for OpenShift and NVIDIA GPU Operator tests.

Given a Prow CI build log, provide a brief TL;DR summary that answers:
1. WHAT failed (be specific - which step, test, or component)
2. WHY it failed (root cause based on error messages)
3. WHAT TO DO (retest if flaky/infra issue, or fix if code problem)

Rules:
- Maximum 3-4 sentences
- Be specific to THIS failure, not generic advice
- If it looks like infrastructure flakiness (connection refused, timeout, quota), say "likely safe to retest"
- If it's a real test/code failure, describe what needs attention
- Don't explain what CI or Prow is"""


def fetch_file_from_gcs(bucket: str, path: str) -> str | None:
    """Fetch a file from GCS (replicates prow-analyzer logic)."""
    url = f"https://storage.googleapis.com/storage/v1/b/{bucket}/o/{urllib.parse.quote(path, safe='')}"
    
    try:
        response = requests.get(url, params={"alt": "media"}, timeout=60)
        response.raise_for_status()
        return response.text
    except requests.RequestException as e:
        logger.error(f"Failed to fetch {path}: {e}")
        return None


def extract_relevant_log(log: str, max_chars: int = 16000) -> str:
    """
    Extract the most relevant portion of a potentially huge log.
    
    Strategy:
    - Keep the beginning (job info, setup)
    - Keep the end (where failures are reported)
    - Truncate the middle if needed
    
    Note: We keep ~16K chars (~4K tokens) to stay within Groq free tier limits
    (6K tokens/min) with room for prompt and response.
    """
    if len(log) <= max_chars:
        return log
    
    # Keep more from the end where failures are typically reported
    beginning_chars = 3000
    ending_chars = max_chars - beginning_chars - 100  # 100 for separator
    
    beginning = log[:beginning_chars]
    ending = log[-ending_chars:]
    
    return f"{beginning}\n\n... [log truncated - {len(log) - max_chars:,} chars omitted] ...\n\n{ending}"


def summarize_with_groq(job_name: str, build_log: str) -> str:
    """Generate a TL;DR summary using Groq (free, no credit card required)."""
    
    api_key = os.environ.get("GROQ_API_KEY")
    if not api_key:
        raise ValueError("GROQ_API_KEY environment variable is required")
    
    client = Groq(api_key=api_key)
    relevant_log = extract_relevant_log(build_log)
    
    logger.info(f"Sending {len(relevant_log):,} chars to Groq for summarization")
    
    response = client.chat.completions.create(
        model="llama-3.3-70b-versatile",  # Free tier model, very capable
        messages=[
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": f"Job: {job_name}\n\nBuild log:\n```\n{relevant_log}\n```\n\nTL;DR:"}
        ],
        max_tokens=500,
        temperature=0.3,  # Lower temperature for more focused output
    )
    
    return response.choices[0].message.content.strip()


def build_log_path(org_repo: str, pr_number: str, job_name: str, build_id: str) -> str:
    """Construct the GCS path for a build log."""
    return f"pr-logs/pull/{org_repo}/{pr_number}/{job_name}/{build_id}/build-log.txt"


def build_prow_url(org_repo: str, pr_number: str, job_name: str, build_id: str) -> str:
    """Construct the Prow UI URL for a build."""
    return (
        f"https://prow.ci.openshift.org/view/gs/{GCS_BUCKET}/"
        f"pr-logs/pull/{org_repo}/{pr_number}/{job_name}/{build_id}"
    )


def format_comment(job_name: str, build_id: str, summary: str, prow_url: str) -> str:
    """Format the PR comment."""
    return f"""## 🔴 CI Failure: `{job_name}`

**TL;DR:** {summary}

---
<sub>🤖 [View full logs]({prow_url}) | Build: `{build_id}`</sub>
"""


def parse_prow_url(url: str) -> dict | None:
    """
    Parse a Prow URL to extract job info.
    
    Expected formats:
    - https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/org_repo/PR/job-name/build-id
    - gs://test-platform-results/pr-logs/pull/org_repo/PR/job-name/build-id
    """
    patterns = [
        r'/pr-logs/pull/([^/]+)/(\d+)/([^/]+)/(\d+)',
        r'pr-logs/pull/([^/]+)/(\d+)/([^/]+)/(\d+)',
    ]
    
    for pattern in patterns:
        match = re.search(pattern, url)
        if match:
            return {
                "org_repo": match.group(1),
                "pr_number": match.group(2),
                "job_name": match.group(3),
                "build_id": match.group(4),
            }
    
    return None


def set_github_output(name: str, value: str):
    """Set a GitHub Actions output variable (handles multiline)."""
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as f:
            # Use delimiter for multiline values
            f.write(f"{name}<<EOF\n{value}\nEOF\n")
    else:
        # Fallback for local testing
        print(f"::set-output name={name}::{value}")


def main():
    """Main entry point."""
    
    # Get job info from environment (set by GitHub Action)
    pr_number = os.environ.get("PR_NUMBER")
    job_name = os.environ.get("JOB_NAME")
    build_id = os.environ.get("BUILD_ID")
    org_repo = os.environ.get("ORG_REPO", DEFAULT_ORG_REPO)
    
    # Alternatively, parse from a Prow URL
    prow_url_input = os.environ.get("PROW_URL")
    if prow_url_input and not all([pr_number, job_name, build_id]):
        parsed = parse_prow_url(prow_url_input)
        if parsed:
            pr_number = parsed["pr_number"]
            job_name = parsed["job_name"]
            build_id = parsed["build_id"]
            org_repo = parsed["org_repo"]
    
    if not all([pr_number, job_name, build_id]):
        logger.error("Missing required parameters. Need PR_NUMBER, JOB_NAME, BUILD_ID or PROW_URL")
        sys.exit(1)
    
    logger.info(f"Analyzing failure for PR #{pr_number}, job: {job_name}, build: {build_id}")
    
    # Fetch the build log
    log_path = build_log_path(org_repo, pr_number, job_name, build_id)
    logger.info(f"Fetching log from gs://{GCS_BUCKET}/{log_path}")
    
    build_log = fetch_file_from_gcs(GCS_BUCKET, log_path)
    
    if not build_log:
        error_msg = f"Could not fetch build log from {log_path}"
        logger.error(error_msg)
        set_github_output("error", error_msg)
        sys.exit(1)
    
    logger.info(f"Fetched {len(build_log):,} bytes of log content")
    
    # Generate AI summary
    try:
        summary = summarize_with_groq(job_name, build_log)
        logger.info(f"Generated summary: {summary[:100]}...")
    except Exception as e:
        error_msg = f"Failed to generate summary: {e}"
        logger.error(error_msg)
        set_github_output("error", error_msg)
        sys.exit(1)
    
    # Format the comment
    prow_url = build_prow_url(org_repo, pr_number, job_name, build_id)
    comment = format_comment(job_name, build_id, summary, prow_url)
    
    # Output for GitHub Actions
    set_github_output("summary", comment)
    set_github_output("pr_number", pr_number)
    
    # Also print for local testing
    print("\n" + "=" * 60)
    print(comment)
    print("=" * 60 + "\n")


if __name__ == "__main__":
    main()
