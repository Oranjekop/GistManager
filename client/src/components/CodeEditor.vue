<script setup>
import * as monaco from 'monaco-editor'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  filename: {
    type: String,
    default: ''
  },
  readOnly: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue'])

const container = ref(null)
let editor
let model
let isApplyingExternalValue = false

function detectLanguage(filename) {
  const ext = filename.split('.').pop()?.toLowerCase()
  const map = {
    js: 'javascript',
    mjs: 'javascript',
    cjs: 'javascript',
    ts: 'typescript',
    vue: 'html',
    json: 'json',
    md: 'markdown',
    yml: 'yaml',
    yaml: 'yaml',
    html: 'html',
    css: 'css',
    scss: 'scss',
    less: 'less',
    xml: 'xml',
    py: 'python',
    java: 'java',
    go: 'go',
    rs: 'rust',
    sh: 'shell',
    ps1: 'powershell',
    sql: 'sql',
    txt: 'plaintext'
  }

  return map[ext] || 'plaintext'
}

const language = computed(() => detectLanguage(props.filename))

function applyTheme() {
  monaco.editor.defineTheme('gist-workbench', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: '', foreground: 'D4D8E3', background: '0F172A' },
      { token: 'comment', foreground: '64748B' },
      { token: 'keyword', foreground: '7DD3FC' },
      { token: 'string', foreground: 'F9A8D4' },
      { token: 'number', foreground: 'FCD34D' }
    ],
    colors: {
      'editor.background': '#0F172A',
      'editorLineNumber.foreground': '#475569',
      'editorLineNumber.activeForeground': '#CBD5E1',
      'editorCursor.foreground': '#F8FAFC',
      'editor.selectionBackground': '#1D4ED866',
      'editor.inactiveSelectionBackground': '#1E293B',
      'editorIndentGuide.background1': '#172033',
      'editorIndentGuide.activeBackground1': '#334155'
    }
  })
}

onMounted(() => {
  applyTheme()
  model = monaco.editor.createModel(props.modelValue, language.value)
  editor = monaco.editor.create(container.value, {
    model,
    theme: 'gist-workbench',
    readOnly: props.readOnly,
    automaticLayout: true,
    wordWrap: 'on',
    fontSize: 14,
    lineHeight: 22,
    fontLigatures: true,
    minimap: { enabled: true },
    smoothScrolling: true,
    roundedSelection: true,
    scrollBeyondLastLine: false,
    padding: {
      top: 18,
      bottom: 18
    }
  })

  editor.onDidChangeModelContent(() => {
    if (isApplyingExternalValue) {
      return
    }

    emit('update:modelValue', model.getValue())
  })
})

watch(
  () => props.modelValue,
  (nextValue) => {
    if (!model || nextValue === model.getValue()) {
      return
    }

    isApplyingExternalValue = true
    model.setValue(nextValue)
    isApplyingExternalValue = false
  }
)

watch(language, (nextLanguage) => {
  if (model) {
    monaco.editor.setModelLanguage(model, nextLanguage)
  }
})

watch(
  () => props.readOnly,
  (nextValue) => {
    editor?.updateOptions({ readOnly: nextValue })
  }
)

onBeforeUnmount(() => {
  editor?.dispose()
  model?.dispose()
})
</script>

<template>
  <div ref="container" class="code-editor-host" />
</template>

<style scoped>
.code-editor-host {
  width: 100%;
  height: 100%;
  min-height: 560px;
  overflow: hidden;
  border-radius: 18px;
}
</style>
