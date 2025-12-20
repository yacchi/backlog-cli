/**
 * 画面中央にポップアップウィンドウを開く
 */
export function openPopupCentered(
  url: string,
  name: string,
  width: number,
  height: number
): Window | null {
  const left = window.screenX + (window.outerWidth - width) / 2
  const top = window.screenY + (window.outerHeight - height) / 2
  return window.open(
    url,
    name,
    `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no`
  )
}
