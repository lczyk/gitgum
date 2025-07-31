source _common.sh
# spellchecker: words gitgum

test_help() {
    assert "gitgum --help"
    assert "gitgum push --help"
    assert "gitgum tree --help"
    assert "gitgum delete --help"
    assert "gitgum status --help"
    assert "gitgum commit --help"
    assert "gitgum switch --help"
}

test_invalid_flag() {
    assert_fails "gitgum --invalid-flag"
    assert_fails "gitgum push --invalid-flag"
    assert_fails "gitgum tree --invalid-flag"
    assert_fails "gitgum delete --invalid-flag"
    assert_fails "gitgum status --invalid-flag"
    assert_fails "gitgum commit --invalid-flag"
    assert_fails "gitgum switch --invalid-flag"
}
