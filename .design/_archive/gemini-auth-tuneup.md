## Fixing up gemini auth

For the gemini harness, we should refine how we determine and apply auth.

In the gemini template, in the home/.gemini/settings.json file, there is a section like:

  "security": {
    "auth": {
      "selectedType": "gemini-api-key"
    }
  },

selectedType may be one of:

gemini-api-key
oauth-personal
vertex-ai

We should allow each of these values for the key "auth_selectedType" in the following places:

the scion settings file under the harness (as a direct key), and provider.(under a gemini harness_overrides)

in the scion-agent.json in a "geminini {}" section

The scion-agent.json value should be filled in upon agent instantiation/creation from a scion settings.json value if present.

### on agent creation

When the agent is created, the value should also be added to the correct place in /home/.gemini/settings.json (the CLI settings file)

Based on this value, the following env keys and volume mounts should also be added (be sure to append to existing, do not just replace) to the scion-agent.json file when the agent is created:

### gemini-api-key

env: {
  GEMINI_API_KEY:"",
}
                      
### oauth-personal

env: {
  GOOGLE_CLOUD_PROJECT:""
}

while this is optional depending on the type of account, it should be added as a blank and as a reminder

### vertex-ai
env: {
  GOOGLE_CLOUD_PROJECT:"",
  GOOGLE_CLOUD_LOCATION:""
},
volumes: {
  /Users/user/.config/gcloud:/home/node/.config/gcloud:ro
}

As a general rule for environment variables, if the key is in a settings file, and the value is not in the environment, a warning should be printed, but not cause a fatal error.

## Clearing up additiona gemini cli specific auth behavior.

There is probably opportunity to simplify auth now that most of this is contained in this setting, and then converted primarily to env and volume config.

Be sure to update tests, and add new ones if needed