import { ElMessageBox, type ElMessageBoxOptions } from 'element-plus'

function mergeClassNames(...classNames: Array<string | undefined>) {
  return classNames.filter(Boolean).join(' ')
}

export function confirmDestructiveAction(
  message: string,
  title: string,
  options: ElMessageBoxOptions = {},
) {
  return ElMessageBox.confirm(message, title, {
    type: 'warning',
    confirmButtonText: '确认',
    cancelButtonText: '取消',
    autofocus: false,
    closeOnClickModal: false,
    ...options,
    customClass: mergeClassNames('recall-message-box', options.customClass),
    confirmButtonClass: mergeClassNames('recall-message-box__danger', options.confirmButtonClass),
  })
}
