#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum commit command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__gitgum_cmd_commit__:-}" ]]; then

HELP_COMMIT="""
Usage: gitgum commit [options]
Options:
  -h, --help      Show this help message
  -n, --dry-run   Perform a dry run without making any changes
This command allows you to commit changes in the current branch.
"""

function gitgum::cmd::commit() {
    gitgum::check_in_git_repo || return 1
    local _parse_flags_help=$HELP_COMMIT
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac

    # Check if there are any changes to commit
    staged_changes=$(git diff --cached --name-only)
    unstaged_changes=$(git diff --name-only)
    new_files=$(git ls-files --others --exclude-standard)
    unstaged_changes=$(echo -e "$unstaged_changes\n$new_files" | sort -u)

    # if we have only staged changes, we can commit
    local needs_to_stage=0
    if [[ -z "$staged_changes" ]] && [[ -z "$unstaged_changes" ]]; then
        echo "No changes to commit."
        return 0
    elif [[ -z "$staged_changes" ]] && [[ -n "$unstaged_changes" ]]; then
        # we have only unstaged changes.
        if _confirm "Only unstaged changes. Stage them all before committing?"; then
            needs_to_stage=1
        else
            return 0
        fi
    elif [[ -n "$staged_changes" ]] && [[ -z "$unstaged_changes" ]]; then
        # we have only staged changes. Nothing to do here.
        true
    else
        # we have both staged and unstaged changes. Maybe we want to stage all changes before committing, or maybe we want to commit only the staged changes.
        echo "You have both staged and unstaged changes."

        local answer=$(_choose "What do you want to do?" \
            "Stage all changes" \
            "Commit only staged changes" \
            "Abort commit")
        case "$answer" in
        "Stage all changes")
            needs_to_stage=1
            ;;
        "Commit only staged changes")
            needs_to_stage=0
            ;;
        "Abort commit")
            return 0
            ;;
        *)
            echo "Unknown option: $answer"
            return 1
            ;;
        esac
    fi

    # Get the commit message
    local commit_message=$(gum input --placeholder "Enter commit message")
    if [[ -z "$commit_message" ]]; then
        echo "No commit message provided. Aborting commit."
        return 1
    fi

    if [[ $needs_to_stage -eq 1 ]]; then
        # Stage all changes
        _maybe_dry_run "git add ."
        _echo_if_not_dry_run "Staged all changes."
    fi

    # Commit the changes
    _maybe_dry_run "git commit -m \"$commit_message\"" ||
        _fatal "Could not commit changes. Please check the output above for details."

    _echo_if_not_dry_run "Committed changes with message: '$commit_message'"

    # Check if there is a remote branch to push to
    local current_branch=$(git rev-parse --abbrev-ref HEAD)
    local remote_branch=$(git branch -r | grep "$current_branch" | sed 's/^[ *]*//')
    if [[ -z "$remote_branch" ]]; then
        echo "No remote branch found for current branch '$current_branch'."
        if _confirm "Do you want to create a remote branch for '$current_branch'?"; then
            _maybe_dry_run "git push -u origin \"$current_branch\""
            _echo_if_not_dry_run "Created remote branch '$current_branch' and set it as the tracking reference."
        else
            echo "Not creating a remote branch. You can push the changes later using 'git push'."
            return 0
        fi
    else
        echo "Remote branch '$remote_branch' found for current branch '$current_branch'."
        # Ask whether to push the changes
        if _confirm "Do you want to push the changes?"; then
            _maybe_dry_run "git push"
            _echo_if_not_dry_run "Pushed changes to the remote repository."
        fi
    fi
}

export __gitgum_cmd_commit__=1
fi
