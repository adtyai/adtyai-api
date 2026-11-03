# Cherry Studio

1. Open **Settings → Model Provider**, click **Add** at the bottom, enter `ADTY AI` as the name and choose **OpenAI** as the provider type.
2. Fill in:

| Field | Value |
|---|---|
| API Key | `sk-your-key` |
| API Host | `{{BASE_URL}}` |

3. Click **Check** to test the key, then turn the provider on.
4. Click **Manage** (or **Add**) under the model list and add the models you want, using names from the [Pricing](/pricing) page, for example `claude-sonnet-4-5` or `gpt-4o-mini`.
5. Pick the model at the top of a chat and start talking.

## Tips

- Enter the API host without `/v1`; Cherry Studio adds `/v1/chat/completions` itself. If requests fail with 404, check that the host does not end with `/v1`.
- Check fails with 401: the key is wrong or disabled. Check fails with 403: the balance has run out or the key is not allowed to use that model. See [Errors and limits](/docs/errors-and-limits).
