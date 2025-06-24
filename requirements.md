# Requirements for the Branch Out Application

An overview of requirements for the branch out application.

## Key Features

* Receive webhooks from the Trunk.io service, and act on them accordingly.
* Log incoming and outgoing requests in a sustainable way
* If Trunk sends a webhook that a test has been quarantined, we should:
  * Find that test in the specified repo and make a GitHub PR to use `t.Skip()` on it.
  * Create a Jira ticket to fix the test.
* If Trunk sends a webhook that a test has been released from quarantine, we should:
  * Find that test in the specified repo and make a GitHub PR to remove any `t.Skip()` calls from it.
  * Find any associated Jira tickets and mark them as complete, along with a comment.
