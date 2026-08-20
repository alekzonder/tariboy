#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

typedef void (*TariboyNotificationActivationCallback)(const char *payload_json);
typedef void (*TariboyNotificationShowCompletion)(int outcome, void *context);

enum {
    TariboyNotificationOutcomeUnavailable = 0,
    TariboyNotificationOutcomeShown = 1,
    TariboyNotificationOutcomeDenied = 2,
};

static NSString *const TariboyActivationKey = @"tariboy_activation";

@interface TariboyNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@property(nonatomic, assign) TariboyNotificationActivationCallback activationCallback;
+ (instancetype)shared;
@end

@implementation TariboyNotificationDelegate
+ (instancetype)shared {
    static TariboyNotificationDelegate *delegate;
    static dispatch_once_t onceToken;
    dispatch_once(&onceToken, ^{
      delegate = [[TariboyNotificationDelegate alloc] init];
    });
    return delegate;
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions options))completionHandler {
    (void)center;
    (void)notification;
    completionHandler(UNNotificationPresentationOptionBanner |
                      UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))completionHandler {
    (void)center;
    if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
        id value = response.notification.request.content.userInfo[TariboyActivationKey];
        if ([value isKindOfClass:[NSString class]] && self.activationCallback != NULL) {
            self.activationCallback([(NSString *)value UTF8String]);
        }
    }
    completionHandler();
}
@end

static void tariboy_post_notification(UNUserNotificationCenter *center,
                                       NSString *identifier,
                                       NSString *title,
                                       NSString *body,
                                       NSString *payload,
                                       TariboyNotificationShowCompletion completion,
                                       void *context) {
    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = title;
    content.body = body;
    content.sound = [UNNotificationSound defaultSound];
    content.userInfo = @{TariboyActivationKey : payload};
    UNNotificationRequest *request =
        [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:nil];
    [center addNotificationRequest:request
             withCompletionHandler:^(NSError *error) {
               completion(error == nil ? TariboyNotificationOutcomeShown
                                       : TariboyNotificationOutcomeUnavailable,
                          context);
             }];
}

void tariboy_notifications_init(TariboyNotificationActivationCallback callback) {
    TariboyNotificationDelegate *delegate = [TariboyNotificationDelegate shared];
    delegate.activationCallback = callback;
    [UNUserNotificationCenter currentNotificationCenter].delegate = delegate;
}

void tariboy_notification_show(const char *identifier,
                               const char *title,
                               const char *body,
                               const char *payload_json,
                               TariboyNotificationShowCompletion completion,
                               void *context) {
    if (identifier == NULL || title == NULL || body == NULL || payload_json == NULL ||
        completion == NULL) {
        if (completion != NULL) {
            completion(TariboyNotificationOutcomeUnavailable, context);
        }
        return;
    }

    NSString *identifierString = [NSString stringWithUTF8String:identifier];
    NSString *titleString = [NSString stringWithUTF8String:title];
    NSString *bodyString = [NSString stringWithUTF8String:body];
    NSString *payloadString = [NSString stringWithUTF8String:payload_json];
    if (identifierString == nil || titleString == nil || bodyString == nil || payloadString == nil) {
        completion(TariboyNotificationOutcomeUnavailable, context);
        return;
    }

    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      if (settings.authorizationStatus == UNAuthorizationStatusDenied) {
          completion(TariboyNotificationOutcomeDenied, context);
          return;
      }
      if (settings.authorizationStatus == UNAuthorizationStatusNotDetermined) {
          [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert |
                                                    UNAuthorizationOptionSound)
                                completionHandler:^(BOOL granted, NSError *error) {
                                  if (!granted) {
                                      completion(error == nil ? TariboyNotificationOutcomeDenied
                                                              : TariboyNotificationOutcomeUnavailable,
                                                 context);
                                      return;
                                  }
                                  tariboy_post_notification(center, identifierString, titleString,
                                                             bodyString, payloadString, completion,
                                                             context);
                                }];
          return;
      }
      tariboy_post_notification(center, identifierString, titleString, bodyString, payloadString,
                                 completion, context);
    }];
}
