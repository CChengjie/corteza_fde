<template>
  <fieldset class="c311-attachment-picker" data-c311-attachment-picker>
    <legend>{{ translate('portal.submit.attachments', 'Attachments') }}</legend>
    <div class="form-group">
      <label for="c311-attachment-file">{{ translate('field.attachmentFile', 'Upload an attachment') }}</label>
      <input
        id="c311-attachment-file"
        class="form-control-file"
        type="file"
        :accept="mediaTypes.join(',')"
        :disabled="disabled || uploading || attachments.length >= maxCount"
        @change="$emit('select', $event)"
      >
      <small class="form-text text-muted">{{ translate('portal.submit.attachmentFileHelp', 'JPEG, PNG, PDF, text, or DOCX up to 10 MB.') }}</small>
    </div>
    <div v-if="uploading" class="progress mb-2" role="progressbar" :aria-valuenow="progress" aria-valuemin="0" aria-valuemax="100" data-c311-upload-progress>
      <div class="progress-bar" :style="{ width: `${progress}%` }">{{ progress }}%</div>
    </div>
    <p v-if="error" class="alert alert-danger" role="alert" data-c311-attachment-error>
      {{ error.message || translate('error.attachmentUpload', 'The attachment could not be uploaded.') }}
      <button v-if="error.retryable" class="btn btn-link p-0 ml-1" type="button" data-c311-action="retry-attachment" @click="$emit('retry')">
        {{ translate('action.retry', 'Try again') }}
      </button>
    </p>
    <p v-if="actionError" class="alert alert-warning" role="alert" data-c311-attachment-action-error>
      {{ actionError.message || translate('error.attachmentAction', 'The attachment could not be opened.') }}
    </p>
    <ul v-if="attachments.length" class="list-unstyled" data-c311-attachment-list>
      <li v-for="attachment in attachments" :key="attachment.attachment_token" class="d-flex align-items-center justify-content-between border-bottom py-2">
        <span>
          <strong data-c311-attachment-name>{{ attachment.filename }}</strong>
          <small class="d-block text-muted">{{ attachment.media_type }} - {{ attachment.size }} bytes</small>
          <small class="d-block text-success" data-c311-attachment-token-status>{{ translate('status.attachmentReady', 'Attachment ready') }}</small>
        </span>
        <span class="d-flex flex-wrap">
          <template v-if="canDownload">
            <button class="btn btn-sm btn-link" type="button" data-c311-action="download-attachment" @click="$emit('download', attachment)">
              {{ translate('action.download', 'Download') }}
            </button>
            <button class="btn btn-sm btn-link" type="button" data-c311-action="preview-attachment" @click="$emit('preview', attachment)">
              {{ translate('action.preview', 'Preview') }}
            </button>
          </template>
          <button class="btn btn-sm btn-link text-danger" type="button" data-c311-action="remove-attachment" @click="$emit('remove', attachment)">
            {{ translate('action.remove', 'Remove') }}
          </button>
        </span>
      </li>
    </ul>
    <p v-if="preview && canDownload" class="alert alert-info" role="status" data-c311-attachment-preview>
      <strong>{{ preview.filename }}</strong>
      <span class="d-block">{{ preview.message }}</span>
      <pre v-if="preview.content_type === 'text/plain' && preview.body" class="mt-2 mb-2" data-c311-attachment-preview-content>{{ preview.body }}</pre>
      <object v-else-if="preview.url" class="d-block w-100 mt-2 mb-2" :data="preview.url" :type="preview.content_type" data-c311-attachment-preview-object>
        <a :href="preview.url" target="_blank" rel="noopener" data-c311-action="open-attachment-preview">{{ translate('action.openPreview', 'Open preview') }}</a>
      </object>
      <button class="btn btn-link p-0" type="button" data-c311-action="close-attachment-preview" @click="$emit('close-preview')">
        {{ translate('action.close', 'Close') }}
      </button>
    </p>
    <small class="form-text text-muted">{{ translate('portal.submit.attachmentHelp', 'Only the returned attachment token is sent with your request.') }}</small>
  </fieldset>
</template>

<script>
import { PORTAL_ATTACHMENT_MEDIA_TYPES } from '@cortezaproject/corteza-js'

export default {
  name: 'C311AttachmentPicker',
  props: {
    attachments: { type: Array, default: () => [] },
    uploading: Boolean,
    progress: { type: Number, default: 0 },
    error: { type: [Object, Error], default: null },
    actionError: { type: [Object, Error], default: null },
    preview: { type: Object, default: null },
    canDownload: Boolean,
    disabled: Boolean,
    maxCount: { type: Number, default: 5 },
  },
  computed: {
    mediaTypes () { return PORTAL_ATTACHMENT_MEDIA_TYPES || [] },
  },
  methods: {
    translate (key, fallback) {
      const translated = this.$t?.(`c311:${key}`)
      return translated && translated !== `c311:${key}` && translated !== key ? translated : fallback
    },
  },
}
</script>
