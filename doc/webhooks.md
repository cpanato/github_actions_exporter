## GitHub Webhooks

## [Workflow Run](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#workflow_run)

* Webhook field `action` is `requested` or `completed`.
  * `requested` has `status=queued` and `conclusion=null`.
  * `completed` can be `failure`, `success`, `cancelled`.

## [Workflow Job](https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#workflow_job)

* Webhook field `action` and `workflow_job[status]` are equal in webhook payloads and include `queued`, `in_progress`, `completed`
* `workflow_job[conclusion]` includes but is not limited to `in_progress`, `queued`, `success`, `failure`, `cancelled`, `skipped`.
* `workflow_job[started_at]` and `workflow_job[completed_at]` are present in all `conclusion`s that are associated with 
  the `completed` `status` i.e `failure`, `success`, `cancelled`, `skipped`.



